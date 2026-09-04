package command_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/command"
	"github.com/JaredTate/nerdgenie/internal/config"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theModelMenu is the part of what "nerdgenie init" printed that runs from the
// question above the menu down to the prompt under it, with the two addresses
// that change from run to run put back to fixed words, so that the menu itself
// can be a golden file.
func theModelMenu(t *testing.T, printed string, daemon string, lmStudio string) []byte {
	t.Helper()
	const question = "Which model should Nerd Genie use?"
	begins := strings.Index(printed, question)
	if begins < 0 {
		t.Fatalf("nerdgenie init never printed the model menu:\n%s", printed)
	}
	ends := strings.Index(printed[begins:], "Type the number")
	if ends < 0 {
		t.Fatalf("the model menu has no prompt under it:\n%s", printed)
	}

	menu := printed[begins : begins+ends]
	menu = strings.ReplaceAll(menu, daemon, "THE-LOCAL-DAEMON")
	menu = strings.ReplaceAll(menu, lmStudio, "THE-LM-STUDIO-SERVER")
	return []byte(menu)
}

// everythingAnswering puts all four of the models that can be found on this
// machine within reach: the local daemon, an LM Studio server, and the two
// subscription programs. It gives back the two addresses, so that a test can
// take them back out of the menu again.
func everythingAnswering(t *testing.T) (string, string) {
	t.Helper()
	folder := noProgramsOnThePath(t)
	putProgramOnThePath(t, folder, contract.ClaudeProgram)
	putProgramOnThePath(t, folder, contract.CodexProgram)
	return runningDaemon(t), runningLMStudio(t, "qwen3-27b-uncensored")
}

func TestTheModelMenuIsInTheOrderTheDesignAsksFor(t *testing.T) {
	home := emptyHome(t)
	daemon, lmStudio := everythingAnswering(t)
	written := &strings.Builder{}

	err := command.Init(context.Background(), command.Setup{
		Home:            home,
		Input:           strings.NewReader("\n1\n"),
		Output:          written,
		LocalAddress:    daemon,
		LMStudioAddress: lmStudio,
	}, nil)
	if err != nil {
		t.Fatalf("nerdgenie init failed with every model answering: %v", err)
	}

	testkit.Golden(t, "model-menu.golden", theModelMenu(t, written.String(), daemon, lmStudio))
}

func TestInitWithYesTakesTheLocalDaemonWhenEveryModelAnswers(t *testing.T) {
	home := emptyHome(t)
	daemon, lmStudio := everythingAnswering(t)

	err := command.Init(context.Background(), command.Setup{
		Home:            home,
		Output:          &strings.Builder{},
		LocalAddress:    daemon,
		LMStudioAddress: lmStudio,
	}, []string{"--yes"})
	if err != nil {
		t.Fatalf("nerdgenie init --yes failed with every model answering: %v", err)
	}

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("the configuration nerdgenie init wrote will not load: %v", err)
	}
	if settings.DefaultModel != contract.LocalModelAlias {
		t.Errorf("nerdgenie init --yes chose %q, and the local daemon is first in the order and was answering", settings.DefaultModel)
	}
}

func TestInitOffersTheLocalModelAndSaysWhatWasNotFoundWhenNothingIsRunning(t *testing.T) {
	home := emptyHome(t)
	noProgramsOnThePath(t)
	written := &strings.Builder{}

	// With nothing found, the menu is the two API keys and then the local model
	// as a last line, which is the third and the way through for a person who
	// has no key at all.
	err := command.Init(context.Background(), command.Setup{
		Home:            home,
		Input:           strings.NewReader("\n3\n"),
		Output:          written,
		LocalAddress:    nothingListening(t),
		LMStudioAddress: nothingListening(t),
	}, nil)
	if err != nil {
		t.Fatalf("nerdgenie init dead-ended a machine with nothing running on it: %v", err)
	}

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("the configuration nerdgenie init wrote will not load: %v", err)
	}
	if settings.DefaultModel != contract.LocalModelAlias {
		t.Errorf("the configuration names %q rather than the local model the last line of the menu offered", settings.DefaultModel)
	}
	for _, wanted := range []string{"was not found on this machine", "start it later"} {
		if !strings.Contains(written.String(), wanted) {
			t.Errorf("nerdgenie init does not print %q above its menu:\n%s", wanted, written)
		}
	}

	configuration := readTheConfiguration(t, home)
	if !strings.Contains(configuration, "was not answering") {
		t.Errorf("config.toml does not say that the server it names was not answering:\n%s", configuration)
	}
}

func TestInitGoesBackToTheMenuWhenNoKeyIsTypedAtThePrompt(t *testing.T) {
	home := emptyHome(t)
	noProgramsOnThePath(t)
	written := &strings.Builder{}
	asked := 0

	// The first answer picks Anthropic, whose key prompt is left empty, and the
	// third line of the menu the second time round is the local model.
	err := command.Init(context.Background(), command.Setup{
		Home:            home,
		Input:           strings.NewReader("\n1\n3\n"),
		Output:          written,
		LocalAddress:    nothingListening(t),
		LMStudioAddress: nothingListening(t),
		AskSecret: func(string) (string, error) {
			asked++
			return "", nil
		},
	}, nil)
	if err != nil {
		t.Fatalf("nerdgenie init gave up when the key prompt was left empty: %v", err)
	}

	if asked != 1 {
		t.Errorf("the key prompt was put %d times, and pressing Enter at it goes back to the menu once", asked)
	}
	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("the configuration nerdgenie init wrote will not load: %v", err)
	}
	if settings.DefaultModel != contract.LocalModelAlias {
		t.Errorf("the configuration names %q rather than the model picked on the second time through the menu", settings.DefaultModel)
	}
	if !strings.Contains(written.String(), "menu again") {
		t.Errorf("nerdgenie init does not say that it is showing the menu again:\n%s", written)
	}
}

// readTheConfiguration reads back the config.toml that "nerdgenie init" wrote.
func readTheConfiguration(t *testing.T, home contract.Home) string {
	t.Helper()
	written, err := os.ReadFile(home.ConfigFile())
	if err != nil {
		t.Fatalf("reading the configuration nerdgenie init wrote failed: %v", err)
	}
	return string(written)
}
