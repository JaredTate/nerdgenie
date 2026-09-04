//go:build live

package context

import (
	stdcontext "context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/clock"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/provider"
)

// This is the proof that "works on any model" is a test rather than a claim. The
// forty-step fixture is played to its last round, the working context is built
// for each of the three real models at that model's own window, and the prompt
// is sent. Nothing here ever skips: a daemon that is down or a program that is
// not signed in is a failure, because the whole point of the live suite is to
// find out that one of them has stopped working.

// The three models the live suite calls, as they are set up on the development
// machine, with the window each one really has.
var liveModels = []contract.ModelAlias{
	{
		Name: contract.LocalModelAlias, Provider: contract.ProviderOpenAI,
		BaseAddress: "http://127.0.0.1:19091/v1", ModelName: "local-coder", ContextLength: 262144,
	},
	{
		Name: "opus on a subscription", Provider: contract.ProviderCommandLine,
		Program: contract.ClaudeProgram, ModelName: "claude-opus-4-8", ContextLength: 200000,
	},
	{
		Name: "gpt on a subscription", Provider: contract.ProviderCommandLine,
		Program: contract.CodexProgram, ModelName: "gpt-5.5", ContextLength: 200000,
	},
}

// liveCallTimeout is how long one real call may take before the test gives up
// and says which model did not answer.
const liveCallTimeout = 5 * time.Minute

// liveOutputCap is what the live suite lets a model write, kept small because
// every one of these calls costs the user money or a graphics card's time.
const liveOutputCap = 1024

// TestTheFixturePromptFitsAndAnswersOnEveryRealModel builds the working context
// for the last round of the fixture on all three models and sends it. The
// numbers it prints are what goes into docs/PROGRESS.md.
func TestTheFixturePromptFitsAndAnswersOnEveryRealModel(t *testing.T) {
	requireTheLocalDaemonIsUp(t)
	run := newFixtureRun(t)
	run.playTo(t, 40)
	home := liveHome(t)
	builder := newTestBuilder(t, Options{Home: home, MaxOutputTokens: liveOutputCap})

	for _, alias := range liveModels {
		t.Run(alias.Name, func(t *testing.T) {
			if alias.Provider == contract.ProviderCommandLine {
				requireTheProgramIsThere(t, alias.Program)
			}
			request, err := builder.Build(t.Context(), run.input(alias.ContextLength))
			if err != nil {
				t.Fatalf("cannot build the working context for %s: %v", alias.Name, err)
			}
			estimated := EstimateRequestTokens(request)
			if estimated+liveOutputCap > alias.ContextLength {
				t.Fatalf("the prompt for %s is about %d tokens and the model holds %d, of which %d are kept for the reply",
					alias.Name, estimated, alias.ContextLength, liveOutputCap)
			}
			sendAndReport(t, alias, home, request, estimated)
		})
	}
}

// sendAndReport makes one real call and prints what it cost.
func sendAndReport(t *testing.T, alias contract.ModelAlias, home contract.Home, request contract.Request, estimated int) {
	t.Helper()
	notes := []string{}
	model, err := provider.New(alias, provider.Options{
		Clock: clock.System(), Home: home,
		Log: func(line string) { notes = append(notes, line) },
	})
	if err != nil {
		t.Fatalf("the model %s could not be set up: %v", alias.Name, err)
	}

	ctx, giveUp := stdcontext.WithTimeout(stdcontext.Background(), liveCallTimeout)
	defer giveUp()
	reply, err := model.Send(ctx, request, nil)
	if err != nil {
		t.Fatalf("the model %s refused the fixture's prompt: %v", alias.Name, err)
	}
	for _, line := range notes {
		t.Logf("%s: %s", alias.Name, line)
	}
	t.Logf("%s: the prompt is about %d tokens by this package's estimate; %s read %d, %d of them cached, and wrote %d, for %.5f dollars, finishing with %q",
		alias.Name, estimated, reply.Model, reply.Usage.InputTokens, reply.Usage.CachedInputTokens,
		reply.Usage.OutputTokens, reply.Usage.CostUSD, reply.Finish)

	if strings.TrimSpace(reply.Text) == "" && len(reply.ToolCalls) == 0 {
		t.Errorf("the model %s answered the fixture's prompt with nothing at all", alias.Name)
	}
	if reply.Usage.InputTokens > 0 && reply.Usage.InputTokens > alias.ContextLength {
		t.Errorf("the model %s read %d tokens and says it holds %d", alias.Name, reply.Usage.InputTokens, alias.ContextLength)
	}
}

// requireTheLocalDaemonIsUp fails when the daemon is not answering and says how
// to start it, because a live test never skips.
func requireTheLocalDaemonIsUp(t *testing.T) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	answer, err := client.Get("http://127.0.0.1:19091/health")
	if err != nil {
		t.Fatalf("the local model daemon on port 19091 is not answering, so start it with ~/llm/igo.sh and try again: %v", err)
	}
	defer answer.Body.Close()
	if answer.StatusCode != http.StatusOK {
		t.Fatalf("the local model daemon answered its health check with %d, want 200", answer.StatusCode)
	}
}

// requireTheProgramIsThere fails with the program's name when it is not
// installed, which is the message the orchestrator needs to fix it.
func requireTheProgramIsThere(t *testing.T, program string) {
	t.Helper()
	if _, err := exec.LookPath(program); err != nil {
		t.Fatalf("the program %s is not on the path, so install it, sign in, and put its folder on PATH: %v", program, err)
	}
}

// liveHome builds the agent's home under a temporary directory by hand, rather
// than with the temporary home from testkit. That one points the HOME variable
// at the temporary directory, and the two subscription programs keep their
// sign-in under the user's real home, so moving HOME would make every live call
// fail as though nobody had ever logged in.
func liveHome(t *testing.T) contract.Home {
	t.Helper()
	home := contract.NewHome(filepath.Join(t.TempDir(), contract.HomeFolderName))
	for _, folder := range home.Folders() {
		if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
			t.Fatalf("the folder %s for the live run could not be made: %v", folder, err)
		}
	}
	writePersonaFile(t, home.SoulFile(), "You are Coeus. You are careful, you say what you are doing, and you never guess.")
	writePersonaFile(t, home.UserFactsFile(), "The user is Jared. He works on DigiByte and prefers short answers.")
	writePersonaFile(t, home.WorldFactsFile(), "DigiByte launched on the tenth of January 2014.")
	return home
}
