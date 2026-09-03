package command_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/command"
	"github.com/JaredTate/coeus/internal/config"
	"github.com/JaredTate/coeus/internal/contract"
)

// runningLMStudio serves the model list an LM Studio server answers with, so
// that detection finds it and reads the name of the model it has loaded.
func runningLMStudio(t *testing.T, loaded string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/models" {
			http.Error(writer, "this fake LM Studio serves only /v1/models", http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(writer, `{"object":"list","data":[{"id":"`+loaded+`"}]}`)
	}))
	t.Cleanup(server.Close)
	return server.URL + "/v1"
}

func TestInitTakesTheModelNameFromTheServerThatAnswered(t *testing.T) {
	home := emptyHome(t)
	noProgramsOnThePath(t)

	err := command.Init(context.Background(), command.Setup{
		Home:            home,
		Output:          &strings.Builder{},
		LocalAddress:    nothingListening(t),
		LMStudioAddress: runningLMStudio(t, "qwen3-27b-uncensored"),
	}, []string{"--yes"})
	if err != nil {
		t.Fatalf("coeus init --yes failed with only LM Studio answering: %v", err)
	}

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("the configuration coeus init wrote will not load: %v", err)
	}
	if settings.DefaultModel != command.LMStudioAlias {
		t.Fatalf("coeus init chose %q rather than the LM Studio server that answered", settings.DefaultModel)
	}
	if settings.Models[0].ModelName != "qwen3-27b-uncensored" {
		t.Errorf("the configuration names the model %q rather than the one the server said it had loaded", settings.Models[0].ModelName)
	}
}

func TestInitAsksAgainWhenAnAnswerWillNotDo(t *testing.T) {
	home := emptyHome(t)
	folder := noProgramsOnThePath(t)
	putProgramOnThePath(t, folder, "signal-cli")
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("reading the home directory failed: %v", err)
	}
	written := &strings.Builder{}

	answers := userHome + "\n~/work\nbanana\n1\nmaybe\nyes\n"
	if err := command.Init(context.Background(), setupWithADaemon(t, home, answers, written), nil); err != nil {
		t.Fatalf("coeus init failed after answers it had to ask about again: %v", err)
	}

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("the configuration coeus init wrote will not load: %v", err)
	}
	wanted := filepath.Join(userHome, "work")
	if len(settings.SandboxRoots) != 1 || settings.SandboxRoots[0] != wanted {
		t.Errorf("the sandbox roots came out %v rather than the folder given on the second try", settings.SandboxRoots)
	}
	if !strings.Contains(written.String(), "type a number") {
		t.Errorf("coeus init did not say what was wrong with the answer to the menu:\n%s", written)
	}
	if !strings.Contains(written.String(), "answer yes or no") {
		t.Errorf("coeus init did not say what was wrong with the answer about Signal:\n%s", written)
	}
}

func TestInitSaysWhenTheAnswersRunOut(t *testing.T) {
	home := emptyHome(t)
	noProgramsOnThePath(t)

	err := command.Init(context.Background(), setupWithADaemon(t, home, "\n", &strings.Builder{}), nil)
	if err == nil {
		t.Fatalf("coeus init carried on after the answers ran out")
	}
	if !strings.Contains(err.Error(), "ran out") {
		t.Errorf("the refusal does not say that the answers ran out: %v", err)
	}
}

func TestInitGivesUpOnAMenuNobodyAnswersProperly(t *testing.T) {
	home := emptyHome(t)
	noProgramsOnThePath(t)

	err := command.Init(context.Background(), setupWithADaemon(t, home, "\nbanana\npear\nplum\n", &strings.Builder{}), nil)
	if err == nil {
		t.Fatalf("coeus init carried on after three answers that were not numbers")
	}
	if !strings.Contains(err.Error(), "no configuration was written") {
		t.Errorf("the refusal does not say what was and was not set up: %v", err)
	}
}

func TestInitGivesUpOnAFolderNobodyNamesProperly(t *testing.T) {
	home := emptyHome(t)
	noProgramsOnThePath(t)
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("reading the home directory failed: %v", err)
	}

	answers := strings.Repeat(userHome+"\n", maxAnswersInATest)
	err = command.Init(context.Background(), setupWithADaemon(t, home, answers, &strings.Builder{}), nil)
	if err == nil {
		t.Fatalf("coeus init took the whole home directory after being told three times that it could not")
	}
	if !strings.Contains(err.Error(), "no configuration was written") {
		t.Errorf("the refusal does not say what was and was not set up: %v", err)
	}
}

// maxAnswersInATest is how many times a test that means to exhaust the tries
// answers a question. It is one more than the code allows, so the test does not
// depend on the exact number.
const maxAnswersInATest = 4

func TestInitGivesUpWhenNoAnswerArrivesAtAll(t *testing.T) {
	home := emptyHome(t)
	noProgramsOnThePath(t)
	neverAnswers, _ := io.Pipe()
	t.Cleanup(func() { _ = neverAnswers.Close() })

	stopped, stop := context.WithCancel(context.Background())
	stop()

	err := command.Init(stopped, command.Setup{
		Home:            home,
		Input:           neverAnswers,
		Output:          &strings.Builder{},
		LocalAddress:    nothingListening(t),
		LMStudioAddress: nothingListening(t),
	}, nil)
	if err == nil {
		t.Fatalf("coeus init waited for ever for an answer that was never going to come")
	}
	if !strings.Contains(err.Error(), "coeus init again") {
		t.Errorf("the refusal does not say to run coeus init again: %v", err)
	}
}

func TestInitAsksForAnAPIKeyThroughTheMaskedPrompt(t *testing.T) {
	home := emptyHome(t)
	noProgramsOnThePath(t)
	asked := ""

	err := command.Init(context.Background(), command.Setup{
		Home:            home,
		Output:          &strings.Builder{},
		LocalAddress:    nothingListening(t),
		LMStudioAddress: nothingListening(t),
		AskSecret: func(prompt string) (string, error) {
			asked = prompt
			return " sk-typed-at-the-prompt ", nil
		},
	}, []string{"--model", command.AnthropicAlias, "--yes"})
	if err != nil {
		t.Fatalf("coeus init with a key at the masked prompt failed: %v", err)
	}
	if !strings.Contains(asked, "not shown") {
		t.Errorf("the prompt does not say that the key is not shown as it is typed: %q", asked)
	}
}

func TestInitRefusesToSetUpAModelWithNoKeyToReachItWith(t *testing.T) {
	broken := errors.New("the prompt was stopped before a secret was entered")
	for _, one := range []struct {
		what      string
		setup     func(setup *command.Setup)
		arguments []string
		says      string
	}{{
		what:      "no terminal to type it in",
		setup:     func(setup *command.Setup) { setup.AskSecret = nil },
		arguments: []string{"--model", command.AnthropicAlias, "--yes"},
		says:      "--api-key-from-env",
	}, {
		what:      "an environment variable with nothing in it",
		arguments: []string{"--model", command.AnthropicAlias, "--api-key-from-env", "A_VARIABLE_WITH_NOTHING_IN_IT", "--yes"},
		says:      "A_VARIABLE_WITH_NOTHING_IN_IT",
	}, {
		what:      "nothing typed at the prompt",
		setup:     func(setup *command.Setup) { setup.AskSecret = func(string) (string, error) { return "  ", nil } },
		arguments: []string{"--model", command.OpenAIAlias, "--yes"},
		says:      "--api-key-from-env",
	}, {
		what:      "a prompt that was stopped",
		setup:     func(setup *command.Setup) { setup.AskSecret = func(string) (string, error) { return "", broken } },
		arguments: []string{"--model", command.OpenAIAlias, "--yes"},
		says:      "nothing was kept",
	}} {
		t.Run(one.what, func(t *testing.T) {
			home := emptyHome(t)
			noProgramsOnThePath(t)
			setup := command.Setup{
				Home:            home,
				Output:          &strings.Builder{},
				LocalAddress:    nothingListening(t),
				LMStudioAddress: nothingListening(t),
				AskSecret:       func(string) (string, error) { return "sk-a-key", nil },
			}
			if one.setup != nil {
				one.setup(&setup)
			}

			err := command.Init(context.Background(), setup, one.arguments)
			if err == nil {
				t.Fatalf("coeus init set a model up with %s", one.what)
			}
			if !strings.Contains(err.Error(), one.says) {
				t.Errorf("the refusal does not say %q: %v", one.says, err)
			}
		})
	}
}

func TestInitRefusesAModelNameItDoesNotKnow(t *testing.T) {
	home := emptyHome(t)
	noProgramsOnThePath(t)

	err := command.Init(context.Background(), command.Setup{
		Home:            home,
		Output:          &strings.Builder{},
		LocalAddress:    nothingListening(t),
		LMStudioAddress: nothingListening(t),
	}, []string{"--model", "nothing-like-this", "--yes"})
	if err == nil {
		t.Fatalf("coeus init took a model name it does not know")
	}
	if !strings.Contains(err.Error(), contract.LocalModelAlias) {
		t.Errorf("the refusal does not list the names it does know: %v", err)
	}
}

func TestInitRefusesAWorkFolderListWithNothingInIt(t *testing.T) {
	home := emptyHome(t)
	noProgramsOnThePath(t)

	err := command.Init(context.Background(), command.Setup{
		Home:            home,
		Output:          &strings.Builder{},
		LocalAddress:    runningDaemon(t),
		LMStudioAddress: nothingListening(t),
	}, []string{"--model", "local", "--work-folder", " , ", "--yes"})
	if err != nil {
		t.Fatalf("coeus init failed on a work-folder list of nothing but commas: %v", err)
	}

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("the configuration coeus init wrote will not load: %v", err)
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("reading the home directory failed: %v", err)
	}
	if len(settings.SandboxRoots) != 1 || settings.SandboxRoots[0] != contract.DefaultSandboxRoots(userHome)[0] {
		t.Errorf("the sandbox roots came out %v rather than the default work folder", settings.SandboxRoots)
	}
}

func TestInitRefusesASignalAnswerThatIsNeitherOnNorOff(t *testing.T) {
	home := emptyHome(t)
	noProgramsOnThePath(t)

	err := command.Init(context.Background(), command.Setup{
		Home:            home,
		Output:          &strings.Builder{},
		LocalAddress:    nothingListening(t),
		LMStudioAddress: nothingListening(t),
	}, []string{"--signal", "maybe"})
	if err == nil {
		t.Fatalf("coeus init took a signal answer that is neither on nor off")
	}
	if !strings.Contains(err.Error(), "--signal on") {
		t.Errorf("the refusal does not say what to write instead: %v", err)
	}
}

func TestInitNeedsSomewhereToPrint(t *testing.T) {
	if err := command.Init(context.Background(), command.Setup{Home: emptyHome(t)}, nil); err == nil {
		t.Fatalf("coeus init ran with nowhere to print its questions")
	}
}
