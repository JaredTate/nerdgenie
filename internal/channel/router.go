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

// Routes are the four things the router needs from the rest of the program.
// cmd/coeus/serve.go supplies all four, which is what keeps this package from
// knowing anything about the agent loop, the command registry, or the skill
// folder.
type Routes struct {
	// FindCommand looks one slash command up by its name without the slash, and
	// says whether there is one. serve.go fills it from the command registry.
	FindCommand func(name string) (contract.Command, bool)
	// FindChannel looks one channel up by the name a message came in under, so
	// that the answer goes back the way the message came. serve.go fills it from
	// the channels it started.
	FindChannel func(name string) (contract.Channel, bool)
	// StartTask hands a message the model has to work on to the agent loop.
	StartTask func(ctx context.Context, message contract.Inbound) error
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

// NewRouter returns a router over the four things serve.go supplies, and refuses
// to be built without any of them, because a router missing one of them would
// drop messages rather than route them.
func NewRouter(routes Routes) (*Router, error) {
	switch {
	case routes.FindCommand == nil:
		return nil, errors.New("the router needs a way to look a command up by name, so pass the command registry's lookup")
	case routes.FindChannel == nil:
		return nil, errors.New("the router needs a way to find the channel a message came from, so pass the lookup over the channels")
	case routes.StartTask == nil:
		return nil, errors.New("the router needs a way to hand work to the agent loop, so pass the function that starts a task")
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

// runCommand looks the command up, refuses it when it may not run on this
// channel, runs it, and sends whatever it answered back the way the message
// came.
func (router *Router) runCommand(ctx context.Context, message contract.Inbound, text string) error {
	where, err := router.channelFor(message)
	if err != nil {
		return err
	}
	name, arguments := SplitCommand(text)

	command, found := router.routes.FindCommand(name)
	if !found {
		return where.Send(ctx, fmt.Sprintf("there is no %s%s command, so type %shelp to see the ones there are", CommandPrefix, shortenedText(name), CommandPrefix))
	}
	if command.TerminalOnly && message.Channel != TerminalChannelName {
		return where.Send(ctx, fmt.Sprintf("the %s%s command works only in the terminal, so run it there", CommandPrefix, command.Name))
	}

	answer, err := command.Run(ctx, arguments, contract.CommandContext{Channel: where})
	if err != nil {
		return errors.Join(
			fmt.Errorf("the %s%s command failed: %w", CommandPrefix, command.Name, err),
			where.Send(ctx, fmt.Sprintf("the %s%s command could not run: %s", CommandPrefix, command.Name, err)))
	}
	if strings.TrimSpace(answer) == "" {
		return nil
	}
	return where.Send(ctx, answer)
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
			fmt.Errorf("the skill %q failed: %w", name, err),
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

// SplitCommand reads a line that starts with a slash into the command's name,
// without the slash and in lower case, and everything the user typed after it.
// A line that is only a slash has an empty name, which no command answers to.
func SplitCommand(text string) (string, string) {
	line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), CommandPrefix))
	name, arguments, _ := strings.Cut(line, " ")
	return strings.ToLower(name), strings.TrimSpace(arguments)
}

// maxTextInAMessage is how much of what the user wrote an error message repeats
// back, so that a very long line cannot fill a screen or a log.
const maxTextInAMessage = 60

// shortenedText cuts what the user wrote down to something an error message can
// carry, and says it was cut.
func shortenedText(text string) string {
	if len(text) <= maxTextInAMessage {
		return text
	}
	return text[:maxTextInAMessage] + "..."
}
