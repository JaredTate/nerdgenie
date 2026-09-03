package channel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// errNoSuchCommand stands in for internal/command's ErrNoSuchCommand, which
// this package must not import. The registry wraps it around the one line it
// answers a line naming no command with.
var errNoSuchCommand = errors.New("there is no command by that name")

// routerHarness is a router with everything around it a test needs to say where
// a message went.
type routerHarness struct {
	router   *Router
	terminal *testkit.FakeChannel
	signal   *testkit.FakeChannel
	skills   *testkit.FakeSkill
	commands map[string]contract.Command
	lines    []string
	tasks    []contract.Inbound
	stops    int
	taskFail error
	stopFail error
}

// newRouterHarness builds a router over two fake channels, a fake skill store,
// a stand-in for the command registry, and a stand-in for the loop's stop.
func newRouterHarness(t *testing.T) *routerHarness {
	t.Helper()
	harness := &routerHarness{
		terminal: testkit.NewFakeChannel(contract.TerminalChannelName),
		signal:   testkit.NewFakeChannel("signal"),
		skills:   testkit.NewFakeSkill(),
		commands: map[string]contract.Command{},
	}
	router, err := NewRouter(Routes{
		RunCommand: harness.runLikeTheRegistry,
		FindChannel: func(name string) (contract.Channel, bool) {
			switch name {
			case contract.TerminalChannelName:
				return harness.terminal, true
			case "signal":
				return harness.signal, true
			default:
				return nil, false
			}
		},
		StartTask: func(_ context.Context, message contract.Inbound) error {
			harness.tasks = append(harness.tasks, message)
			return harness.taskFail
		},
		StopTask: func(context.Context) error {
			harness.stops++
			return harness.stopFail
		},
		Skills: harness.skills,
	})
	if err != nil {
		t.Fatalf("building the router failed: %v", err)
	}
	harness.router = router
	return harness
}

// runLikeTheRegistry stands in for internal/command's Registry.Run: it writes
// down the line exactly as it was handed over, reads the name off the front the
// way the registry does, refuses a terminal-only command anywhere else with a
// reply rather than an error, and answers a line naming no command it holds
// with errNoSuchCommand.
func (harness *routerHarness) runLikeTheRegistry(ctx context.Context, line string, where contract.CommandContext) (string, error) {
	harness.lines = append(harness.lines, line)
	name, arguments := splitLikeTheRegistry(line)

	command, found := harness.commands[name]
	if !found {
		return "", fmt.Errorf("there is no command called /%s, so type /help for the list: %w", name, errNoSuchCommand)
	}
	if command.TerminalOnly && (where.Channel == nil || where.Channel.Name() != contract.TerminalChannelName) {
		return fmt.Sprintf("the /%s command works only in the terminal, so run it there", command.Name), nil
	}
	return command.Run(ctx, arguments, where)
}

// splitLikeTheRegistry reads a line apart into a name and its arguments by the
// registry's rules: every leading slash and space taken off, the name ending at
// the first space, and no change of case.
func splitLikeTheRegistry(line string) (string, string) {
	trimmed := strings.TrimLeft(line, " \t\r\n/")
	end := strings.IndexAny(trimmed, " \t\r\n")
	if end < 0 {
		return trimmed, ""
	}
	return trimmed[:end], strings.TrimSpace(trimmed[end:])
}

// route sends one message through the router and fails the test when the router
// could not carry it.
func (harness *routerHarness) route(t *testing.T, text string) Kind {
	t.Helper()
	kind, err := harness.router.Route(context.Background(), anInbound(text))
	if err != nil {
		t.Fatalf("routing %q failed: %v", text, err)
	}
	return kind
}

func TestASlashCommandIsRunOnTheChannelItCameFrom(t *testing.T) {
	harness := newRouterHarness(t)
	var ranWith string
	var ranOn string
	harness.commands["tasks"] = contract.Command{
		Name: "tasks",
		Help: "shows what is running, waiting, or done",
		Run: func(_ context.Context, arguments string, where contract.CommandContext) (string, error) {
			ranWith, ranOn = arguments, where.Channel.Name()
			return "task 17 is running", nil
		},
	}

	if kind := harness.route(t, "  /tasks 17 back 3 "); kind != KindCommand {
		t.Errorf("the router made %q a %s, want a command", "/tasks", kind)
	}
	if ranWith != "17 back 3" {
		t.Errorf("the command was run with %q, want %q", ranWith, "17 back 3")
	}
	if ranOn != contract.TerminalChannelName {
		t.Errorf("the command was run on the channel %q, want %q", ranOn, contract.TerminalChannelName)
	}
	if sent := harness.terminal.Sent(); len(sent) != 1 || sent[0] != "task 17 is running" {
		t.Errorf("the terminal carried %v, want the one line the command returned", sent)
	}
	if len(harness.tasks) != 0 {
		t.Errorf("a command was also sent to the loop as a task: %v", harness.tasks)
	}
}

func TestTheWholeLineGoesToTheCommandRunnerUnsplitAndUnchanged(t *testing.T) {
	harness := newRouterHarness(t)
	harness.commands["Status"] = contract.Command{
		Name: "Status",
		Help: "shows the model, the cost, the jobs, and the health",
		Run:  func(context.Context, string, contract.CommandContext) (string, error) { return "", nil },
	}

	// The registry is the one place a typed line is read apart into a name and
	// its arguments. A second reading here, with rules of its own, is what let
	// the router run a command the registry would not have found and skip the
	// registry's own refusals, so the router hands the line over as it stands.
	if kind := harness.route(t, "  /Status  the rest  "); kind != KindCommand {
		t.Errorf("the router made a command a %s, want a command", kind)
	}
	if len(harness.lines) != 1 {
		t.Fatalf("the command runner was given %d lines, want 1", len(harness.lines))
	}
	if harness.lines[0] != "/Status  the rest" {
		t.Errorf("the command runner was given %q, want the line as it was typed with only the space around it taken off", harness.lines[0])
	}
}

func TestAnUnknownCommandAnswersWithOneLineNamingHelp(t *testing.T) {
	harness := newRouterHarness(t)

	kind, err := harness.router.Route(context.Background(), anInbound("/wibble now"))
	if kind != KindCommand {
		t.Errorf("the router made an unknown command a %s, want a command", kind)
	}
	if err == nil {
		t.Error("an unknown command was routed with nothing to report, and the caller has to be told the line did nothing")
	}
	sent := harness.terminal.Sent()
	if len(sent) != 1 {
		t.Fatalf("an unknown command was answered with %d messages, want 1", len(sent))
	}
	if strings.Contains(sent[0], "\n") {
		t.Errorf("the answer is more than one line: %q", sent[0])
	}
	if !strings.Contains(sent[0], "/help") {
		t.Errorf("the answer reads %q, and it has to name /help", sent[0])
	}
	if !strings.Contains(sent[0], "wibble") {
		t.Errorf("the answer reads %q, and it has to name what was typed", sent[0])
	}
	if len(harness.tasks) != 0 {
		t.Errorf("an unknown command was sent to the loop as a task: %v", harness.tasks)
	}
}

func TestAStopNoCommandAnswersToStopsTheRunningTask(t *testing.T) {
	harness := newRouterHarness(t)

	// Escape at the terminal sends /stop while a task runs. Until the agent
	// loop registers a command by that name, nothing answers to it, and a
	// person pressing Escape must still stop the task.
	if kind := harness.route(t, "/stop"); kind != KindCommand {
		t.Errorf("the router made /stop a %s, want a command", kind)
	}
	if harness.stops != 1 {
		t.Fatalf("the loop was asked to stop %d times, want once", harness.stops)
	}
	sent := harness.terminal.Sent()
	if len(sent) != 1 || !strings.Contains(sent[0], "stop") {
		t.Errorf("the user was told %v, want one line saying the task was asked to stop", sent)
	}
	if len(harness.tasks) != 0 {
		t.Errorf("a stop was sent to the loop as a task: %v", harness.tasks)
	}
}

func TestAStopACommandDoesAnswerToIsLeftToThatCommand(t *testing.T) {
	harness := newRouterHarness(t)
	ran := 0
	harness.commands["stop"] = contract.Command{
		Name: "stop",
		Help: "stops what the agent is doing",
		Run: func(context.Context, string, contract.CommandContext) (string, error) {
			ran++
			return "stopping", nil
		},
	}

	if kind := harness.route(t, "/stop"); kind != KindCommand {
		t.Errorf("the router made /stop a %s, want a command", kind)
	}
	if ran != 1 {
		t.Errorf("the registered /stop ran %d times, want once", ran)
	}
	if harness.stops != 0 {
		t.Errorf("the loop's own stop was also called %d times, and the registered command is the one that stops the task", harness.stops)
	}
}

func TestAStopTheLoopCannotCarryOutTellsTheUserAndSaysSo(t *testing.T) {
	harness := newRouterHarness(t)
	harness.stopFail = errors.New("no task is running, so there is nothing to stop")

	kind, err := harness.router.Route(context.Background(), anInbound("/stop"))
	if err == nil {
		t.Fatal("a stop the loop could not carry out was routed without an error, and the caller has to be told")
	}
	if kind != KindCommand {
		t.Errorf("a stop that failed was a %s, want a command", kind)
	}
	sent := harness.terminal.Sent()
	if len(sent) != 1 || !strings.Contains(sent[0], "nothing to stop") {
		t.Errorf("the user was told %v, want the reason the loop gave", sent)
	}
}

func TestAnythingElseGoesToTheLoopAsATask(t *testing.T) {
	harness := newRouterHarness(t)
	harness.skills.Add(contract.SkillSummary{Name: "weekly-note", Description: "posts it"}, "steps", "weekly note")

	if kind := harness.route(t, "book me a flight to Denver"); kind != KindTask {
		t.Errorf("the router made an ordinary message a %s, want a task", kind)
	}
	if len(harness.tasks) != 1 || harness.tasks[0].Text != "book me a flight to Denver" {
		t.Fatalf("the loop was given %v, want the one message", harness.tasks)
	}
	if runs := harness.skills.Runs(); len(runs) != 0 {
		t.Errorf("a skill ran on a message that triggered none: %v", runs)
	}
	if sent := harness.terminal.Sent(); len(sent) != 0 {
		t.Errorf("the router answered a task itself: %v", sent)
	}
}

func TestACommandThatOnlyWorksInTheTerminalIsRefusedAnywhereElse(t *testing.T) {
	harness := newRouterHarness(t)
	ran := false
	harness.commands["vault"] = contract.Command{
		Name:         "vault",
		Help:         "shows and manages the secret store",
		TerminalOnly: true,
		Run: func(_ context.Context, _ string, _ contract.CommandContext) (string, error) {
			ran = true
			return "the vault holds two secrets", nil
		},
	}

	// The refusal is the command runner's, and it only reaches the user because
	// the router goes through the runner rather than around it.
	fromSignal := anInbound("/vault list")
	fromSignal.Channel = "signal"
	kind, err := harness.router.Route(context.Background(), fromSignal)
	if err != nil {
		t.Fatalf("routing the vault command from Signal failed: %v", err)
	}
	if kind != KindCommand {
		t.Errorf("the router made a terminal-only command a %s, want a command", kind)
	}
	if ran {
		t.Error("the vault command ran through Signal, and it works only in the terminal")
	}
	sent := harness.signal.Sent()
	if len(sent) != 1 || !strings.Contains(sent[0], "terminal") {
		t.Errorf("Signal was told %v, want one line saying the command works only in the terminal", sent)
	}
}

func TestACommandThatFailsTellsTheUserAndSaysSo(t *testing.T) {
	harness := newRouterHarness(t)
	harness.commands["undo"] = contract.Command{
		Name: "undo",
		Help: "puts back the last turn's file changes",
		Run: func(_ context.Context, _ string, _ contract.CommandContext) (string, error) {
			return "", errors.New("there is nothing in the log to put back, so there is nothing to undo")
		},
	}

	kind, err := harness.router.Route(context.Background(), anInbound("/undo"))
	if err == nil {
		t.Fatal("a command that failed was routed without an error, and the caller has to be told")
	}
	if kind != KindCommand {
		t.Errorf("a failed command was a %s, want a command", kind)
	}
	sent := harness.terminal.Sent()
	if len(sent) != 1 || !strings.Contains(sent[0], "nothing to undo") {
		t.Errorf("the user was told %v, want the reason the command gave", sent)
	}
}

func TestAMessageFromAChannelTheProgramDoesNotKnowIsRefused(t *testing.T) {
	harness := newRouterHarness(t)
	harness.skills.Add(contract.SkillSummary{Name: "weekly-note", Description: "posts it"}, "steps", "weekly note")

	for _, text := range []string{"/help", "post the weekly note"} {
		fromNowhere := anInbound(text)
		fromNowhere.Channel = "telegram"
		if _, err := harness.router.Route(context.Background(), fromNowhere); err == nil {
			t.Errorf("%q came from a channel the program does not know and was routed anyway, and the answer would have gone nowhere", text)
		}
	}
}

func TestAVeryLongChannelNameIsCutDownBeforeTheRouterRepeatsItBack(t *testing.T) {
	harness := newRouterHarness(t)
	fromNowhere := anInbound("/help")
	fromNowhere.Channel = strings.Repeat("wibble", 40)

	_, err := harness.router.Route(context.Background(), fromNowhere)
	if err == nil {
		t.Fatal("a message from a channel the program does not know was routed anyway")
	}
	if strings.Contains(err.Error(), fromNowhere.Channel) {
		t.Errorf("the router repeated the channel name back in full in %q, and a very long one has to be cut down first", err)
	}
}

func TestTheRouterRefusesToBeBuiltWithoutItsPieces(t *testing.T) {
	whole := Routes{
		RunCommand:  func(context.Context, string, contract.CommandContext) (string, error) { return "", nil },
		FindChannel: func(string) (contract.Channel, bool) { return nil, false },
		StartTask:   func(context.Context, contract.Inbound) error { return nil },
		StopTask:    func(context.Context) error { return nil },
		Skills:      testkit.NewFakeSkill(),
	}
	missing := map[string]Routes{
		"the command runner": {
			FindChannel: whole.FindChannel, StartTask: whole.StartTask,
			StopTask: whole.StopTask, Skills: whole.Skills,
		},
		"the channel lookup": {
			RunCommand: whole.RunCommand, StartTask: whole.StartTask,
			StopTask: whole.StopTask, Skills: whole.Skills,
		},
		"the way to start a task": {
			RunCommand: whole.RunCommand, FindChannel: whole.FindChannel,
			StopTask: whole.StopTask, Skills: whole.Skills,
		},
		"the way to stop a task": {
			RunCommand: whole.RunCommand, FindChannel: whole.FindChannel,
			StartTask: whole.StartTask, Skills: whole.Skills,
		},
		"the skill store": {
			RunCommand: whole.RunCommand, FindChannel: whole.FindChannel,
			StartTask: whole.StartTask, StopTask: whole.StopTask,
		},
	}
	for what, routes := range missing {
		if _, err := NewRouter(routes); err == nil {
			t.Errorf("a router was built without %s", what)
		}
	}
	if _, err := NewRouter(whole); err != nil {
		t.Errorf("a router with everything it needs was refused: %v", err)
	}
}
