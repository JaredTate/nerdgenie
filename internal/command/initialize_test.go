package command_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/command"
	"github.com/JaredTate/nerdgenie/internal/config"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/vault"
)

// signalNextStep is the closing line that offers Signal. The doctor's own
// warning about a missing signal-cli also names the command, so a test that
// wants to know whether Signal was offered has to look for this whole line.
const signalNextStep = "coeus signal link  link Coeus to your Signal account"

// emptyHome points the HOME and XDG_CONFIG_HOME variables at folders with
// nothing in them, which is what a machine looks like before "coeus init" has
// ever run.
func emptyHome(t *testing.T) contract.Home {
	t.Helper()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(userHome, ".config"))
	return contract.NewHome(filepath.Join(userHome, contract.HomeFolderName))
}

// noProgramsOnThePath empties the PATH, so that a test decides for itself which
// of claude, codex, and signal-cli this machine appears to have.
func noProgramsOnThePath(t *testing.T) string {
	t.Helper()
	folder := t.TempDir()
	t.Setenv("PATH", folder)
	return folder
}

// putProgramOnThePath writes an executable of that name into the folder the
// PATH points at, so that detection finds it.
func putProgramOnThePath(t *testing.T, folder string, name string) {
	t.Helper()
	path := filepath.Join(folder, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("writing the fake program %s failed: %v", path, err)
	}
}

// runningDaemon serves the health endpoint the local model daemon answers, and
// gives back the base address a configuration would name.
func runningDaemon(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/health" {
			http.Error(writer, "this fake daemon serves only /health", http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(writer, `{"status":"ok"}`)
	}))
	t.Cleanup(server.Close)
	return server.URL + "/v1"
}

// nothingListening gives back the address of a server that has been shut down,
// so that a probe of it fails at once rather than waiting.
func nothingListening(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))
	server.Close()
	return server.URL + "/v1"
}

// setupWithADaemon is the setup an interactive test uses: an empty home, a
// daemon that answers, no LM Studio, and answers read from the text given.
func setupWithADaemon(t *testing.T, home contract.Home, answers string, written *strings.Builder) command.Setup {
	t.Helper()
	return command.Setup{
		Home:            home,
		Input:           strings.NewReader(answers),
		Output:          written,
		LocalAddress:    runningDaemon(t),
		LMStudioAddress: nothingListening(t),
		AskSecret: func(_ string) (string, error) {
			return "the-key-typed-at-the-masked-prompt", nil
		},
	}
}

func TestInitOnAnEmptyHomeAnswersOnAPipeAndLeavesAConfigurationThatLoads(t *testing.T) {
	home := emptyHome(t)
	noProgramsOnThePath(t)
	written := &strings.Builder{}

	if err := command.Init(context.Background(), setupWithADaemon(t, home, "\n1\n", written), nil); err != nil {
		t.Fatalf("coeus init failed: %v", err)
	}

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("the configuration coeus init wrote will not load: %v", err)
	}
	if settings.DefaultModel != contract.LocalModelAlias {
		t.Errorf("the configuration names %q as the model rather than the daemon that answered", settings.DefaultModel)
	}
	if asked := strings.Count(written.String(), "?"); asked > 6 {
		t.Errorf("coeus init asked %d questions and the limit is six:\n%s", asked, written)
	}
	report := config.Doctor(context.Background(), home)
	if report.Verdict() == config.Trouble {
		t.Errorf("the doctor found something broken after coeus init:\n%s", report)
	}
}

func TestInitMakesTheWholeLayoutTheWorkFolderAndThePersonaFiles(t *testing.T) {
	home := emptyHome(t)
	noProgramsOnThePath(t)

	if err := command.Init(context.Background(), setupWithADaemon(t, home, "\n1\n", &strings.Builder{}), nil); err != nil {
		t.Fatalf("coeus init failed: %v", err)
	}

	for _, folder := range home.Folders() {
		about, err := os.Stat(folder)
		if err != nil {
			t.Errorf("the folder %s was not made: %v", folder, err)
			continue
		}
		if about.Mode().Perm() != contract.HomeFolderMode {
			t.Errorf("the folder %s has mode %04o rather than %04o", folder, about.Mode().Perm(), contract.HomeFolderMode)
		}
	}

	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("reading the home directory failed: %v", err)
	}
	for _, work := range contract.DefaultSandboxRoots(userHome) {
		if about, err := os.Stat(work); err != nil || !about.IsDir() {
			t.Errorf("the work folder %s was not made: %v", work, err)
		}
	}

	for _, path := range []string{home.SoulFile(), home.UserFactsFile(), home.WorldFactsFile()} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("the persona file %s was not written: %v", path, err)
			continue
		}
		if len(content) == 0 {
			t.Errorf("the persona file %s was written empty, and each one explains itself in a line", path)
		}
	}
}

func TestInitTakesEveryAnswerAsAFlagWithNoTerminalAtAll(t *testing.T) {
	home := emptyHome(t)
	noProgramsOnThePath(t)
	work := filepath.Join(t.TempDir(), "Code")
	written := &strings.Builder{}

	err := command.Init(context.Background(), command.Setup{
		Home:            home,
		Output:          written,
		LocalAddress:    runningDaemon(t),
		LMStudioAddress: nothingListening(t),
	}, []string{"--model", "local", "--work-folder", work, "--signal", "off", "--yes"})
	if err != nil {
		t.Fatalf("coeus init with every answer as a flag failed: %v", err)
	}

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("the configuration coeus init wrote will not load: %v", err)
	}
	if len(settings.SandboxRoots) != 1 || settings.SandboxRoots[0] != work {
		t.Errorf("the sandbox roots came out %v rather than the folder the flag named", settings.SandboxRoots)
	}
	if strings.Contains(written.String(), signalNextStep) {
		t.Errorf("coeus init offered Signal after being told to switch it off:\n%s", written)
	}
}

func TestInitWithYesAloneTakesTheFirstModelItDetects(t *testing.T) {
	home := emptyHome(t)
	folder := noProgramsOnThePath(t)
	putProgramOnThePath(t, folder, contract.ClaudeProgram)

	err := command.Init(context.Background(), command.Setup{
		Home:            home,
		Output:          &strings.Builder{},
		LocalAddress:    runningDaemon(t),
		LMStudioAddress: nothingListening(t),
	}, []string{"--yes"})
	if err != nil {
		t.Fatalf("coeus init --yes failed: %v", err)
	}

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("the configuration coeus init wrote will not load: %v", err)
	}
	if settings.DefaultModel != contract.LocalModelAlias {
		t.Errorf("coeus init --yes chose %q rather than the daemon, which is the first thing it detects", settings.DefaultModel)
	}
	if len(settings.FallbackChain) != 1 || settings.FallbackChain[0] != contract.ClaudeProgram {
		t.Errorf("the fallback chain came out %v rather than the other model that was detected", settings.FallbackChain)
	}
}

func TestInitPutsAnAPIKeyInTheVaultAndWritesOnlyAReferenceToIt(t *testing.T) {
	home := emptyHome(t)
	noProgramsOnThePath(t)
	t.Setenv("A_KEY_FOR_THE_TEST", "sk-the-key-itself")

	err := command.Init(context.Background(), command.Setup{
		Home:            home,
		Output:          &strings.Builder{},
		LocalAddress:    nothingListening(t),
		LMStudioAddress: nothingListening(t),
	}, []string{"--model", "anthropic", "--api-key-from-env", "A_KEY_FOR_THE_TEST", "--yes"})
	if err != nil {
		t.Fatalf("coeus init with a key from the environment failed: %v", err)
	}

	written, err := os.ReadFile(home.ConfigFile())
	if err != nil {
		t.Fatalf("reading the configuration failed: %v", err)
	}
	if strings.Contains(string(written), "sk-the-key-itself") {
		t.Fatalf("coeus init wrote the key itself into config.toml")
	}
	if !strings.Contains(string(written), contract.SecretReferencePrefix+"anthropic") {
		t.Errorf("config.toml does not refer to the key in the vault:\n%s", written)
	}

	opened, err := vault.Open(home, testkit.NewFakeClock(theStartOfTime))
	if err != nil {
		t.Fatalf("opening the vault coeus init made failed: %v", err)
	}
	defer func() { _ = opened.Close() }()
	held, err := opened.Resolve(context.Background(), contract.SecretReferencePrefix+"anthropic")
	if err != nil {
		t.Fatalf("the vault does not hold the key coeus init was given: %v", err)
	}
	if held.Password != "sk-the-key-itself" {
		t.Errorf("the vault holds a different key from the one the environment named")
	}
}

func TestInitRefusesToWorkInTheWholeHomeDirectoryAndSaysWhy(t *testing.T) {
	home := emptyHome(t)
	noProgramsOnThePath(t)
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("reading the home directory failed: %v", err)
	}

	err = command.Init(context.Background(), command.Setup{
		Home:            home,
		Output:          &strings.Builder{},
		LocalAddress:    runningDaemon(t),
		LMStudioAddress: nothingListening(t),
	}, []string{"--model", "local", "--work-folder", userHome, "--yes"})
	if err == nil {
		t.Fatalf("coeus init took the whole home directory as a folder to work in")
	}
	if !strings.Contains(err.Error(), contract.HomeFolderName) {
		t.Errorf("the refusal does not say what would have been inside the fence: %v", err)
	}
}

func TestInitSaysWhatToDoWhenItDetectsNothingAndIsToldNotToAsk(t *testing.T) {
	home := emptyHome(t)
	noProgramsOnThePath(t)

	err := command.Init(context.Background(), command.Setup{
		Home:            home,
		Output:          &strings.Builder{},
		LocalAddress:    nothingListening(t),
		LMStudioAddress: nothingListening(t),
	}, []string{"--yes"})
	if err == nil {
		t.Fatalf("coeus init --yes claimed to set up a model when it found none")
	}
	if !strings.Contains(err.Error(), "--model") {
		t.Errorf("the refusal does not say which flag to use: %v", err)
	}
}

func TestInitPrintsTheCommandsANewUserNeeds(t *testing.T) {
	home := emptyHome(t)
	folder := noProgramsOnThePath(t)
	putProgramOnThePath(t, folder, "signal-cli")
	written := &strings.Builder{}

	if err := command.Init(context.Background(), setupWithADaemon(t, home, "\n1\ny\n", written), nil); err != nil {
		t.Fatalf("coeus init failed: %v", err)
	}

	for _, wanted := range []string{signalNextStep, "/help", "/tasks", "coeus doctor"} {
		if !strings.Contains(written.String(), wanted) {
			t.Errorf("coeus init does not tell the user about %q:\n%s", wanted, written)
		}
	}
}

func TestInitOnAHomeThatIsAlreadySetUpChangesNothing(t *testing.T) {
	home := emptyHome(t)
	noProgramsOnThePath(t)
	daemon := runningDaemon(t)

	first := command.Setup{Home: home, Output: &strings.Builder{}, LocalAddress: daemon, LMStudioAddress: nothingListening(t)}
	if err := command.Init(context.Background(), first, []string{"--model", "local", "--yes"}); err != nil {
		t.Fatalf("the first coeus init failed: %v", err)
	}
	before := treeSnapshot(t, home.Root)

	written := &strings.Builder{}
	second := command.Setup{Home: home, Output: written, LocalAddress: daemon, LMStudioAddress: nothingListening(t)}
	if err := command.Init(context.Background(), second, nil); err != nil {
		t.Fatalf("the second coeus init failed: %v", err)
	}

	if after := treeSnapshot(t, home.Root); after != before {
		t.Errorf("the second coeus init changed the home folder:\n--- before ---\n%s\n--- after ---\n%s", before, after)
	}
	if !strings.Contains(written.String(), "--reset-config") {
		t.Errorf("coeus init does not say how to write the configuration again:\n%s", written)
	}
}

func TestInitWritesTheConfigurationAgainWhenAskedTo(t *testing.T) {
	home := emptyHome(t)
	folder := noProgramsOnThePath(t)
	putProgramOnThePath(t, folder, contract.CodexProgram)
	daemon := runningDaemon(t)

	setup := command.Setup{Home: home, Output: &strings.Builder{}, LocalAddress: daemon, LMStudioAddress: nothingListening(t)}
	if err := command.Init(context.Background(), setup, []string{"--model", "local", "--yes"}); err != nil {
		t.Fatalf("the first coeus init failed: %v", err)
	}
	if err := command.Init(context.Background(), setup, []string{"--model", contract.CodexProgram, "--yes", "--reset-config"}); err != nil {
		t.Fatalf("coeus init --reset-config failed: %v", err)
	}

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("the configuration coeus init wrote will not load: %v", err)
	}
	if settings.DefaultModel != contract.CodexProgram {
		t.Errorf("coeus init --reset-config left the model as %q", settings.DefaultModel)
	}
}

func TestInitRefusesAFlagItDoesNotUnderstand(t *testing.T) {
	home := emptyHome(t)
	noProgramsOnThePath(t)

	err := command.Init(context.Background(), command.Setup{
		Home:            home,
		Output:          &strings.Builder{},
		LocalAddress:    nothingListening(t),
		LMStudioAddress: nothingListening(t),
	}, []string{"--nothing-like-this"})
	if err == nil {
		t.Fatalf("coeus init took a flag it does not understand")
	}
	if _, statErr := os.Stat(home.Root); statErr == nil {
		t.Errorf("coeus init made the home folder before reading its own flags")
	}
}

// treeSnapshot writes down every path under a folder with its mode and its
// contents, so that a test can say a second run changed nothing byte for byte.
func treeSnapshot(t *testing.T, root string) string {
	t.Helper()
	written := &strings.Builder{}
	err := filepath.Walk(root, func(path string, about os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		written.WriteString(path + " " + about.Mode().String() + "\n")
		if about.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		written.Write(content)
		written.WriteString("\n")
		return nil
	})
	if err != nil {
		t.Fatalf("walking the home folder failed: %v", err)
	}
	return written.String()
}
