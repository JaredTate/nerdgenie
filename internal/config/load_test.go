package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/config"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// writeConfig puts a configuration file in the home folder and returns the home,
// which is what almost every test here starts with.
func writeConfig(t testing.TB, document string) contract.Home {
	t.Helper()
	home := testkit.NewTempHome(t)
	if err := os.WriteFile(home.ConfigFile(), []byte(document), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the configuration file for the test: %v", err)
	}
	return home
}

func TestAnEmptyFileGivesTheDefaultsTheContractProvides(t *testing.T) {
	home := writeConfig(t, "")

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("loading an empty configuration file failed, and an empty file is a valid configuration: %v", err)
	}

	wanted := contract.DefaultConfig()
	if !reflect.DeepEqual(settings.Models, wanted.Models) {
		t.Errorf("the model aliases are %+v, want the shipped default %+v", settings.Models, wanted.Models)
	}
	if settings.DefaultModel != wanted.DefaultModel {
		t.Errorf("the default model is %q, want %q", settings.DefaultModel, wanted.DefaultModel)
	}
	if len(settings.FallbackChain) != 0 {
		t.Errorf("the fallback chain is %v, want it empty on a fresh install", settings.FallbackChain)
	}
	if settings.SignalAccount != "" {
		t.Errorf("the Signal account is %q, want it empty until coeus signal link runs", settings.SignalAccount)
	}
	if settings.SearchServerAddress != "" {
		t.Errorf("the search server address is %q, want it empty so the web tool reads DuckDuckGo", settings.SearchServerAddress)
	}
	if settings.HandoffTimeout != wanted.HandoffTimeout {
		t.Errorf("the handoff timeout is %s, want %s", settings.HandoffTimeout, wanted.HandoffTimeout)
	}
	if !reflect.DeepEqual(settings.Caps, wanted.Caps) {
		t.Errorf("the caps are %+v, want the design's numbers %+v", settings.Caps, wanted.Caps)
	}
	if !reflect.DeepEqual(settings.MemoryCaps, wanted.MemoryCaps) {
		t.Errorf("the memory caps are %+v, want %+v", settings.MemoryCaps, wanted.MemoryCaps)
	}
	if !reflect.DeepEqual(settings.AskMeFirst, contract.DefaultAskMeFirst()) {
		t.Errorf("the ask-me-first list is %v, want the three entries the design ships %v", settings.AskMeFirst, contract.DefaultAskMeFirst())
	}
	if len(settings.PermissionRules) != 0 {
		t.Errorf("the permission rules are %+v, want none on a fresh install", settings.PermissionRules)
	}
}

func TestTheLocalAliasDefaultsToTheLlamaServerDaemon(t *testing.T) {
	home := writeConfig(t, "")

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("loading an empty configuration file failed: %v", err)
	}
	if len(settings.Models) != 1 {
		t.Fatalf("a fresh install ships %d aliases, want exactly the local one", len(settings.Models))
	}

	local := settings.Models[0]
	checks := []struct {
		what string
		got  string
		want string
	}{
		{"name", local.Name, contract.LocalModelAlias},
		{"provider", string(local.Provider), string(contract.ProviderOpenAI)},
		{"base address", local.BaseAddress, "http://127.0.0.1:19091/v1"},
		{"model name", local.ModelName, "local-coder"},
		{"key reference", local.KeyReference, ""},
	}
	for _, check := range checks {
		if check.got != check.want {
			t.Errorf("the local alias %s is %q, want %q", check.what, check.got, check.want)
		}
	}
	if local.ContextLength != 262144 {
		t.Errorf("the local alias holds %d tokens, want 262144", local.ContextLength)
	}
}

func TestAMissingFileIsTheSameAsAnEmptyOne(t *testing.T) {
	home := testkit.NewTempHome(t)

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("loading a home folder with no configuration file failed, and a missing file means the defaults: %v", err)
	}
	if settings.DefaultModel != contract.DefaultConfig().DefaultModel {
		t.Errorf("the default model is %q, want the shipped default", settings.DefaultModel)
	}
}

func TestThePathsThatDependOnTheHomeFolderAreFilledIn(t *testing.T) {
	home := writeConfig(t, "")

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("loading an empty configuration file failed: %v", err)
	}

	if want := home.BrowserProfile("default"); settings.BrowserProfilePath != want {
		t.Errorf("the browser profile is %q, want the default profile in the home folder, %q", settings.BrowserProfilePath, want)
	}
	if want := home.BackupsFolder(); settings.BackupPath != want {
		t.Errorf("the backup path is %q, want the backups folder in the home folder, %q", settings.BackupPath, want)
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("the test cannot find the user's home directory: %v", err)
	}
	if want := contract.DefaultSandboxRoots(userHome); !reflect.DeepEqual(settings.SandboxRoots, want) {
		t.Errorf("the sandbox roots are %v, want the contract's default %v", settings.SandboxRoots, want)
	}
}

func TestAFileOverridesOnlyTheKeysItSets(t *testing.T) {
	home := writeConfig(t, `
signal_account = "+15125550123"
search_server_address = "https://search.example.com"

[caps]
rounds_per_task = 12
`)

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("loading a configuration file that sets three keys failed: %v", err)
	}

	if settings.SignalAccount != "+15125550123" {
		t.Errorf("the Signal account is %q, want the one the file sets", settings.SignalAccount)
	}
	if settings.SearchServerAddress != "https://search.example.com" {
		t.Errorf("the search server address is %q, want the one the file sets", settings.SearchServerAddress)
	}
	if settings.Caps.RoundsPerTask != 12 {
		t.Errorf("the rounds per task is %d, want the 12 the file sets", settings.Caps.RoundsPerTask)
	}
	if wanted := contract.DefaultConfig().Caps.QueuedMessages; settings.Caps.QueuedMessages != wanted {
		t.Errorf("the queued-message cap is %d, want the default %d, because the file did not set it",
			settings.Caps.QueuedMessages, wanted)
	}
}

// TestABudgetOfZeroIsAcceptedAndMeansNoLimit is the user's rule at the file:
// the three budgets may be written as zero, which is what they are when left
// out, and a budget the user does set still loads as what they wrote.
func TestABudgetOfZeroIsAcceptedAndMeansNoLimit(t *testing.T) {
	off, err := config.Load(writeConfig(t, "\n[caps]\nrounds_per_task = 0\ntime_per_task = \"0s\"\ntime_per_turn = 0\n"))
	if err != nil {
		t.Fatalf("a file that writes the three budgets as zero was refused, and zero means no limit: %v", err)
	}
	if off.Caps.RoundsPerTask != 0 || off.Caps.TimePerTask != 0 || off.Caps.TimePerTurn != 0 {
		t.Errorf("the budgets loaded as %d rounds, %s per task, and %s per turn, want all three at zero",
			off.Caps.RoundsPerTask, off.Caps.TimePerTask, off.Caps.TimePerTurn)
	}
	if off.Caps.TimePerTool != contract.DefaultConfig().Caps.TimePerTool {
		t.Errorf("the time per tool is %s, want the default the file did not touch", off.Caps.TimePerTool)
	}

	set, err := config.Load(writeConfig(t, "\n[caps]\nrounds_per_task = 40\ntime_per_task = \"30m\"\ntime_per_turn = \"5m\"\n"))
	if err != nil {
		t.Fatalf("a file that sets the three budgets was refused: %v", err)
	}
	if set.Caps.RoundsPerTask != 40 || set.Caps.TimePerTask != 30*time.Minute || set.Caps.TimePerTurn != 5*time.Minute {
		t.Errorf("the budgets loaded as %d rounds, %s per task, and %s per turn, want 40, 30m, and 5m",
			set.Caps.RoundsPerTask, set.Caps.TimePerTask, set.Caps.TimePerTurn)
	}
}

func TestALengthOfTimeMayBeWrittenAsAStringOrAsNanoseconds(t *testing.T) {
	forEachWay := map[string]string{
		"a string":     `handoff_timeout = "90s"`,
		"nanoseconds":  "handoff_timeout = 90000000000",
		"a whole hour": `handoff_timeout = "1h30m"`,
	}
	wanted := map[string]time.Duration{
		"a string":     90 * time.Second,
		"nanoseconds":  90 * time.Second,
		"a whole hour": 90 * time.Minute,
	}
	for way, document := range forEachWay {
		t.Run(way, func(t *testing.T) {
			home := writeConfig(t, document+"\n")
			settings, err := config.Load(home)
			if err != nil {
				t.Fatalf("loading a handoff timeout written as %s failed: %v", way, err)
			}
			if settings.HandoffTimeout != wanted[way] {
				t.Errorf("the handoff timeout is %s, want %s", settings.HandoffTimeout, wanted[way])
			}
		})
	}
}

func TestATableArrayReplacesTheShippedModelAliases(t *testing.T) {
	home := writeConfig(t, `
default_model = "cloud"

[[models]]
name = "cloud"
provider = "cli"
program = "claude"
model_name = "opus"
context_length = 200000

[[models]]
name = "local"
provider = "openai"
base_address = "http://127.0.0.1:19091/v1"
model_name = "local-coder"
context_length = 262144
`)

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("loading two model aliases failed: %v", err)
	}
	if len(settings.Models) != 2 {
		t.Fatalf("the file names two aliases and %d came back, so the file did not replace the shipped one", len(settings.Models))
	}
	if settings.Models[0].Name != "cloud" || settings.Models[0].Program != contract.ClaudeProgram {
		t.Errorf("the first alias is %+v, want the cloud alias run through the claude program", settings.Models[0])
	}
}

func TestLoadNamesTheConfigurationFileInEveryError(t *testing.T) {
	home := writeConfig(t, "default_model = 7\n")

	_, err := config.Load(home)
	if err == nil {
		t.Fatal("a number where the default model's name belongs was accepted, want it refused")
	}
	if !strings.Contains(err.Error(), filepath.Base(home.ConfigFile())) {
		t.Errorf("the error is %q, want it to name the file the mistake is in", err)
	}
}

func TestAConfigurationFileFarTooBigIsRefusedRatherThanRead(t *testing.T) {
	home := testkit.NewTempHome(t)
	huge := strings.Repeat("# a comment line that says nothing at all\n", 40000)
	if len(huge) <= config.MaxConfigBytes {
		t.Fatalf("this test needs a document over %d bytes and built one of %d", config.MaxConfigBytes, len(huge))
	}
	if err := os.WriteFile(home.ConfigFile(), []byte(huge), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the oversized configuration file: %v", err)
	}

	_, err := config.Load(home)
	if err == nil {
		t.Fatal("a configuration file of over a megabyte was read, want it refused before anything is parsed")
	}
	if !strings.Contains(err.Error(), "settings file") {
		t.Errorf("the error is %q, want it to say the file is not a settings file", err)
	}

	if _, err := config.Parse(home, []byte(huge)); err == nil {
		t.Fatal("parsing over a megabyte of text was allowed, want it refused")
	}
}

func TestAConfigurationFileThisAccountCannotReadSaysSo(t *testing.T) {
	home := testkit.NewTempHome(t)
	if err := os.Mkdir(home.ConfigFile(), contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot put a folder where the configuration file belongs: %v", err)
	}

	_, err := config.Load(home)
	if err == nil {
		t.Fatal("a folder where the configuration file belongs was accepted, want it refused")
	}
	if !strings.Contains(err.Error(), home.ConfigFile()) {
		t.Errorf("the error is %q, want it to name the path it could not read", err)
	}
}
