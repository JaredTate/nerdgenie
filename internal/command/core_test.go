package command_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/command"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theStartOfTime is the moment every test in this package starts its fake clock
// at, so that nothing here ever reads the machine's own clock.
var theStartOfTime = time.Date(2026, time.September, 2, 14, 3, 0, 0, time.UTC)

// threeAliases is a configuration naming the three models the tests choose
// between, which is what "/model" checks an answer against.
func threeAliases() contract.Config {
	settings := contract.DefaultConfig()
	settings.Models = append(settings.Models,
		contract.ModelAlias{Name: "claude", Provider: contract.ProviderCommandLine, Program: contract.ClaudeProgram, ModelName: "claude-opus-4-8", ContextLength: 200000},
		contract.ModelAlias{Name: "codex", Provider: contract.ProviderCommandLine, Program: contract.CodexProgram, ModelName: "gpt-5.5", ContextLength: 200000})
	return settings
}

// runOne registers the core commands and runs one line through the registry in
// the terminal, which is how a test sees exactly what a user would.
func runOne(t *testing.T, deps command.Deps, line string) string {
	t.Helper()
	registry := command.NewRegistry()
	for _, one := range command.New(registry, deps).All() {
		if err := registry.Register(one); err != nil {
			t.Fatalf("registering %s failed: %v", one.Name, err)
		}
	}
	answer, err := registry.Run(context.Background(), line, contract.CommandContext{Channel: terminalChannel()})
	if err != nil {
		t.Fatalf("running %q failed: %v", line, err)
	}
	return answer
}

func TestHelpListsTheTenCoreCommands(t *testing.T) {
	answer := runOne(t, command.Deps{}, "/help")

	for _, name := range []string{"/help", "/status", "/model", "/new", "/sessions", "/approve", "/deny", "/pause", "/resume", "/undo"} {
		if !strings.Contains(answer, name+" ") {
			t.Errorf("the help listing leaves out %s:\n%s", name, answer)
		}
	}
}

func TestStatusShowsTheModelTheCostTheJobsTheAnswersWaitingAndTheHealth(t *testing.T) {
	clock := testkit.NewFakeClock(theStartOfTime)
	jobs := testkit.NewFakeJob(clock)
	campaign, err := jobs.Create(context.Background(), contract.NewJob{Ask: "run the DigiByte anniversary campaign", Why: "mark the anniversary"})
	if err != nil {
		t.Fatalf("creating the campaign job failed: %v", err)
	}
	for _, text := range []string{"post the anniversary tweet", "write the summary"} {
		if _, err := jobs.AddTask(context.Background(), contract.NewTask{JobID: campaign, Text: text}); err != nil {
			t.Fatalf("adding a task to the campaign job failed: %v", err)
		}
	}
	weekly, err := jobs.Create(context.Background(), contract.NewJob{Ask: "post the weekly update", Why: "keep people informed"})
	if err != nil {
		t.Fatalf("creating the weekly job failed: %v", err)
	}
	if err := jobs.Pause(context.Background(), weekly); err != nil {
		t.Fatalf("pausing the weekly job failed: %v", err)
	}

	signal := testkit.NewFakeChannel("signal")
	signal.SetHealth(false, "signal-cli is not running")

	answer := runOne(t, command.Deps{
		Settings:     threeAliases(),
		Jobs:         jobs,
		CurrentModel: func() string { return "local" },
		CostSoFar: func() contract.CostLine {
			return contract.CostLine{InputTokens: 6100, CachedInputTokens: 5200, OutputTokens: 400}
		},
		PendingPreviews: func(_ context.Context) ([]contract.Preview, error) {
			return []contract.Preview{{ID: "3", Title: "post the anniversary tweet", Body: "DigiByte turns twelve today."}}, nil
		},
		Channels: func() []contract.Channel { return []contract.Channel{terminalChannel(), signal} },
	}, "/status")

	testkit.Golden(t, "status.golden", []byte(answer))
}

func TestStatusOnAFreshProgramSaysThereIsNothingYet(t *testing.T) {
	answer := runOne(t, command.Deps{
		Settings:     contract.DefaultConfig(),
		Jobs:         testkit.NewFakeJob(testkit.NewFakeClock(theStartOfTime)),
		CurrentModel: func() string { return contract.LocalModelAlias },
		CostSoFar:    func() contract.CostLine { return contract.CostLine{} },
		PendingPreviews: func(_ context.Context) ([]contract.Preview, error) {
			return nil, nil
		},
		Channels: func() []contract.Channel { return []contract.Channel{terminalChannel()} },
	}, "/status")

	testkit.Golden(t, "status-fresh.golden", []byte(answer))
}

func TestStatusSaysWhatItCouldNotAskFor(t *testing.T) {
	answer := runOne(t, command.Deps{Settings: contract.DefaultConfig()}, "/status")

	if !strings.Contains(answer, "model:") {
		t.Errorf("the status report says nothing at all when nothing is wired up:\n%s", answer)
	}
	for _, missing := range []string{"jobs", "channels"} {
		if !strings.Contains(answer, missing) {
			t.Errorf("the status report leaves out the %s line when nothing is wired up:\n%s", missing, answer)
		}
	}
}

func TestModelShowsTheOneInUseAndTheAliasesToChooseFrom(t *testing.T) {
	answer := runOne(t, command.Deps{
		Settings:     threeAliases(),
		CurrentModel: func() string { return "local" },
	}, "/model")

	if !strings.Contains(answer, "the model is local") {
		t.Errorf("the model command does not say which model is in use: %q", answer)
	}
	for _, name := range []string{"local", "claude", "codex"} {
		if !strings.Contains(answer, name) {
			t.Errorf("the model command leaves out the alias %q: %q", name, answer)
		}
	}
}

func TestModelSetsAnAliasTheConfigurationNames(t *testing.T) {
	chosen := "local"
	answer := runOne(t, command.Deps{
		Settings:     threeAliases(),
		CurrentModel: func() string { return chosen },
		SetModel: func(alias string) error {
			chosen = alias
			return nil
		},
	}, "/model claude")

	if chosen != "claude" {
		t.Errorf("the model was left as %q rather than set to claude", chosen)
	}
	if !strings.Contains(answer, "claude") {
		t.Errorf("the model command does not say what it set: %q", answer)
	}
}

func TestModelRefusesAnAliasTheConfigurationDoesNotName(t *testing.T) {
	chosen := "local"
	answer := runOne(t, command.Deps{
		Settings:     threeAliases(),
		CurrentModel: func() string { return chosen },
		SetModel: func(alias string) error {
			chosen = alias
			return nil
		},
	}, "/model nothing-like-this")

	if chosen != "local" {
		t.Fatalf("the model was set to %q, which config.toml does not name", chosen)
	}
	if !strings.Contains(answer, "nothing-like-this") || !strings.Contains(answer, "claude") {
		t.Errorf("the refusal does not name what was asked for and what there is: %q", answer)
	}
}

func TestModelSaysWhenNothingCanSetIt(t *testing.T) {
	registry := command.NewRegistry()
	for _, one := range command.New(registry, command.Deps{Settings: threeAliases()}).All() {
		if err := registry.Register(one); err != nil {
			t.Fatalf("registering %s failed: %v", one.Name, err)
		}
	}

	if _, err := registry.Run(context.Background(), "/model claude", contract.CommandContext{Channel: terminalChannel()}); err == nil {
		t.Fatalf("the model command claimed to set the model with nothing wired up to set it")
	}
}

// TestStatusSaysHowHardTheModelInUseThinks holds the half of the think setting
// a person sees without asking for it: the status line names the model and,
// when a level has been set for it, how hard it is thinking. A model left at
// the provider's own default says nothing extra, which is what the golden
// files above hold.
func TestStatusSaysHowHardTheModelInUseThinks(t *testing.T) {
	settings := threeAliases()
	for at := range settings.Models {
		if settings.Models[at].Name == "claude" {
			settings.Models[at].Think = contract.ThinkHigh
		}
	}

	answer := runOne(t, command.Deps{
		Settings:     settings,
		CurrentModel: func() string { return "claude" },
	}, "/status")

	if !strings.Contains(answer, "model: claude, thinking at high") {
		t.Errorf("the status report does not say how hard the model is thinking:\n%s", answer)
	}

	plain := runOne(t, command.Deps{
		Settings:     settings,
		CurrentModel: func() string { return "codex" },
	}, "/status")
	if !strings.Contains(plain, "model: codex\n") {
		t.Errorf("the status report says something about thinking for a model left at its provider's own default:\n%s", plain)
	}
}
