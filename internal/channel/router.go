package channel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// CommandPrefix is the character that makes a message a command rather than
// something for the model.
const CommandPrefix = "/"

// StopCommandName is the command a screen sends when the person presses Escape
// while a task is running.
const StopCommandName = "stop"

// Kind is what the router decided one message was.
type Kind string

const (
	// KindCommand is a message that started with a slash.
	KindCommand Kind = "command"
	// KindSkill is a message whose words triggered a saved skill.
	KindSkill Kind = "skill"
	// KindTask is a message the model has to work on.
	KindTask Kind = "task"
)

// Routes are the five things the router needs from the rest of the program.
// cmd/coeus/serve.go supplies all five, which is what keeps this package from
// knowing anything about the agent loop, the command registry, or the skill
// folder.
type Routes struct {
	// RunCommand runs the command a typed line names and returns the reply to
	// send back. serve.go fills it from the command registry's own Run, so that
	// the registry is the one place a line is read apart into a name and its
	// arguments, and the one place a terminal-only command is refused. A line
	// naming no command the program holds comes back as an error whose text is
	// written for the user to read.
	RunCommand func(ctx context.Context, line string, where contract.CommandContext) (string, error)
	// FindChannel looks one channel up by the name a message came in under, so
	// that the answer goes back the way the message came. serve.go fills it from
	// the channels it started.
	FindChannel func(name string) (contract.Channel, bool)
	// StartTask hands a message the model has to work on to the agent loop.
	StartTask func(ctx context.Context, message contract.Inbound) error
	// StopTask stops the task the agent loop is running. It is what a /stop that
	// no command answers to falls back to, so that Escape at the terminal stops
	// the work whether or not the loop has registered a command of that name.
	StopTask func(ctx context.Context) error
	// Skills is the skill store, asked about every message that is not a
	// command, so that a saved procedure runs without calling the model.
	Skills contract.Skill
}

// Router decides what each message off the queue is and hands it to the right
// place: a slash command run on the channel it came from, a saved skill run
// without the model, or work for the agent loop.
type Router struct {
	routes Routes
}

// NewRouter returns a router over the five things serve.go supplies, and refuses
// to be built without any of them, because a router missing one of them would
// drop messages rather than route them.
func NewRouter(routes Routes) (*Router, error) {
	switch {
	case routes.RunCommand == nil:
		return nil, errors.New("the router needs a way to run a command, so pass the command registry's Run")
	case routes.FindChannel == nil:
		return nil, errors.New("the router needs a way to find the channel a message came from, so pass the lookup over the channels")
	case routes.StartTask == nil:
		return nil, errors.New("the router needs a way to hand work to the agent loop, so pass the function that starts a task")
	case routes.StopTask == nil:
		return nil, errors.New("the router needs a way to stop the running task, so pass the function that stops the agent loop")
	case routes.Skills == nil:
		return nil, errors.New("the router needs the skill store to tell a trigger from a task, so pass the skill store")
	}
	return &Router{routes: routes}, nil
}

// Route decides what one message is and hands it on, returning which of the
// three kinds it was. A message that starts with a slash is a command, run on
// the channel it came from; otherwise the skill store is asked whether the words
// trigger a saved skill, which then runs without the model; anything else is
// work for the agent loop.
//
// It returns an error for trouble it hit on the way, and the kind it returns
// still says where the message went. A skill store that cannot answer is trouble
// of that sort: the message goes to the loop as a task, because a shortcut that
// is broken must never lose the user's words.
func (router *Router) Route(ctx context.Context, message contract.Inbound) (Kind, error) {
	text := strings.TrimSpace(message.Text)
	if strings.HasPrefix(text, CommandPrefix) {
		return KindCommand, router.runCommand(ctx, message, text)
	}

	match, err := router.routes.Skills.Match(ctx, text)
	if err != nil {
		return KindTask, errors.Join(
			fmt.Errorf("cannot tell whether %q triggers a skill, so it went to the model as a task: %w", shortenedText(text), err),
			router.routes.StartTask(ctx, message))
	}
	if match.Matched {
		return KindSkill, router.runSkill(ctx, message, match.Name, text)
	}
	return KindTask, router.routes.StartTask(ctx, message)
}

// runCommand hands the whole line to the one command runner the program has and
// sends whatever it answered back the way the message came. The line is handed
// over as it stands, because the registry is the one place a line is read apart
// into a name and its arguments, and a second reading here with rules of its own
// would run commands the registry would not have found and skip the refusals it
// makes.
//
// A line the runner could not carry out is told to the user in the runner's own
// words, which is where the line naming /help for an unknown command comes from.
// The one exception is /stop: a stop no command answers to falls back to
// stopping the running task, because Escape must stop the work whether or not
// the agent loop has registered a command by that name.
func (router *Router) runCommand(ctx context.Context, message contract.Inbound, text string) error {
	where, err := router.channelFor(message)
	if err != nil {
		return err
	}

	answer, err := router.routes.RunCommand(ctx, text, contract.CommandContext{Channel: where})
	if err != nil {
		if namesTheStopCommand(text) {
			return router.stopTheTask(ctx, where)
		}
		return errors.Join(
			fmt.Errorf("the command %q failed, and the user has been told why: %w", shortenedText(text), err),
			where.Send(ctx, fmt.Sprintf("that command could not run: %s", err)))
	}
	if strings.TrimSpace(answer) == "" {
		return nil
	}
	return where.Send(ctx, answer)
}

// stopTheTask asks the agent loop to stop what it is running and says so on the
// channel the stop came from.
func (router *Router) stopTheTask(ctx context.Context, where contract.Channel) error {
	if err := router.routes.StopTask(ctx); err != nil {
		return errors.Join(
			fmt.Errorf("the running task could not be stopped, and the user has been told why: %w", err),
			where.Send(ctx, fmt.Sprintf("the running task could not be stopped: %s", err)))
	}
	return where.Send(ctx, "the running task has been asked to stop")
}

// namesTheStopCommand says whether a typed line asks for the stop command. It
// reads the name the way internal/command's registry reads it, with every
// leading slash and space taken off, the name ending at the first space, and no
// change of case, so that the two can never disagree about which line names
// /stop.
func namesTheStopCommand(text string) bool {
	name := strings.TrimLeft(text, " \t\r\n"+CommandPrefix)
	if end := strings.IndexAny(name, " \t\r\n"); end >= 0 {
		name = name[:end]
	}
	return name == StopCommandName
}

// runSkill runs the saved skill whose trigger words fired and sends what it
// answered back the way the message came. No model is called.
func (router *Router) runSkill(ctx context.Context, message contract.Inbound, name string, text string) error {
	where, err := router.channelFor(message)
	if err != nil {
		return err
	}

	answer, err := router.routes.Skills.Run(ctx, name, text)
	if err != nil {
		return errors.Join(
			fmt.Errorf("the skill %q failed, and the user has been told why: %w", name, err),
			where.Send(ctx, fmt.Sprintf("the %s skill could not run: %s", name, err)))
	}
	if strings.TrimSpace(answer) == "" {
		return nil
	}
	return where.Send(ctx, answer)
}

// channelFor finds the channel a message came through, which is where its answer
// goes.
func (router *Router) channelFor(message contract.Inbound) (contract.Channel, error) {
	where, found := router.routes.FindChannel(message.Channel)
	if !found {
		return nil, fmt.Errorf("the message came through the channel %q, which is not one this agent is running, so the answer would have nowhere to go", shortenedText(message.Channel))
	}
	return where, nil
}

// maxTextInAMessage is how many letters of what the user wrote an error message
// repeats back, so that a very long line cannot fill a screen or a log.
const maxTextInAMessage = 60

// shortenedText cuts what the user wrote down to something an error message can
// carry, and says it was cut. It counts letters rather than bytes and never
// cuts one in half, because half a letter is not a letter and would reach the
// screen as a question mark in a box.
func shortenedText(text string) string {
	letters := []rune(text)
	if len(letters) <= maxTextInAMessage {
		return text
	}
	return string(letters[:maxTextInAMessage]) + "..."
}
