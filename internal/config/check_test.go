package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/config"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// theMarker is written as a comment on the line each test document gets wrong,
// so that a test says where the mistake is by pointing at it rather than by
// counting lines.
const theMarker = "# HERE"

// markedLine returns the line number the marker is on, which is the line the
// problem must name.
func markedLine(t testing.TB, document string) int {
	t.Helper()
	for offset, text := range strings.Split(document, "\n") {
		if strings.Contains(text, theMarker) {
			return offset + 1
		}
	}
	t.Fatalf("this test document has no %s comment, so it does not say which line is wrong", theMarker)
	return 0
}

// refuses loads a document that is wrong in one place and returns the problem,
// failing the test when the document was accepted or the error is of another
// kind altogether.
func refuses(t testing.TB, document string) config.Problem {
	t.Helper()
	home := writeConfig(t, document)
	_, err := config.Load(home)
	if err == nil {
		t.Fatalf("this configuration was accepted, and it should not have been:\n%s", document)
	}
	var problem config.Problem
	if !errors.As(err, &problem) {
		t.Fatalf("the error is %v of type %T, want a config.Problem naming the key and the line", err, err)
	}
	return problem
}

// A valid model alias, which the table below perturbs one line at a time.
const oneGoodAlias = `
[[models]]
name = "local"
provider = "openai"
base_address = "http://127.0.0.1:19091/v1"
model_name = "local-coder"
context_length = 262144
`

// badField is one configuration file that is wrong in one place, with the key
// the problem must name. The line it must name is the one carrying the marker.
type badField struct {
	what     string
	document string
	key      string
}

// eachIsRefused loads every document in the table and asserts that the problem
// names the key and the marked line.
func eachIsRefused(t *testing.T, table []badField) {
	t.Helper()
	for _, field := range table {
		t.Run(field.what, func(t *testing.T) {
			problem := refuses(t, field.document)
			if problem.Key != field.key {
				t.Errorf("the problem names the key %q, want %q", problem.Key, field.key)
			}
			if want := markedLine(t, field.document); problem.Line != want {
				t.Errorf("the problem names line %d, want line %d", problem.Line, want)
			}
			if len(strings.Fields(problem.Advice)) < 6 {
				t.Errorf("the advice is %q, want a sentence saying what is wrong and what to do", problem.Advice)
			}
			if !strings.Contains(problem.Error(), field.key) {
				t.Errorf("the printed problem is %q, want it to name the key", problem.Error())
			}
		})
	}
}

func TestEveryFieldSetToTheWrongTypeIsRefusedWithItsKeyAndItsLine(t *testing.T) {
	eachIsRefused(t, fieldsOfTheWrongType())
}

// fieldsOfTheWrongType is one configuration file per field, each giving that
// field a value of a type it cannot hold.
func fieldsOfTheWrongType() []badField {
	return []badField{
		{"the default model as a number", "\ndefaultmodel = 7 " + theMarker + "\n", "default_model"},
		{"the fallback chain as one string", "\nfallbackchain = \"cloud\" " + theMarker + "\n", "fallback_chain"},
		{"the Signal account as a number", "\nsignalaccount = 15125550123 " + theMarker + "\n", "signal_account"},
		{"the browser profile as a list", "\nbrowserprofilepath = [\"a\"] " + theMarker + "\n", "browser_profile_path"},
		{"the sandbox roots as a number", "\nsandboxroots = 3 " + theMarker + "\n", "sandbox_roots"},
		{"the backup path as a boolean", "\nbackuppath = true " + theMarker + "\n", "backup_path"},
		{"the search server as a number", "\nsearchserveraddress = 9 " + theMarker + "\n", "search_server_address"},
		{"the handoff timeout as nonsense", "\nhandofftimeout = \"ninety\" " + theMarker + "\n", "handoff_timeout"},
		{"a round count as a word", "\n[caps]\nroundspertask = \"many\" " + theMarker + "\n", "caps.rounds_per_task"},
		{"a length of time as a list", "\n[caps]\ntimepertask = [1] " + theMarker + "\n", "caps.time_per_task"},
		{"a memory cap as a boolean", "\n[memory_caps]\nuserfactsbytes = true " + theMarker + "\n", "memory_caps.user_facts_bytes"},
		{"a context length with a fraction", "\n[[models]]\ncontextlength = 1.5 " + theMarker + "\n", "models.context_length"},
	}
}

func TestEveryFieldSetToAnImpossibleValueIsRefusedWithItsKeyAndItsLine(t *testing.T) {
	eachIsRefused(t, impossibleCaps())
	eachIsRefused(t, impossibleSettings())
	eachIsRefused(t, impossibleModelAliases())
}

// impossibleCaps is one configuration file per cap, each setting a limit that
// leaves the agent no room to work at all.
func impossibleCaps() []badField {
	return []badField{
		{"no rounds in a task", "\n[caps]\nroundspertask = 0 " + theMarker + "\n", "caps.rounds_per_task"},
		{"no queued messages", "\n[caps]\nqueuedmessages = -1 " + theMarker + "\n", "caps.queued_messages"},
		{"no room in a tool result", "\n[caps]\ntooloutputbytes = 0 " + theMarker + "\n", "caps.tool_output_bytes"},
		{"no window for repeated calls", "\n[caps]\nidenticalcallwindow = 0 " + theMarker + "\n", "caps.identical_call_window"},
		{"no time for a task", "\n[caps]\ntimepertask = \"0s\" " + theMarker + "\n", "caps.time_per_task"},
		{"time running backwards for a tool", "\n[caps]\ntimepertool = \"-1m\" " + theMarker + "\n", "caps.time_per_tool"},
		{"no time for a turn", "\n[caps]\ntimeperturn = 0 " + theMarker + "\n", "caps.time_per_turn"},
		{"no room for world facts", "\n[memory_caps]\nworldfactsbytes = 0 " + theMarker + "\n", "memory_caps.world_facts_bytes"},
		{"no room for user facts", "\n[memory_caps]\nuserfactsbytes = -8 " + theMarker + "\n", "memory_caps.user_facts_bytes"},
		{"no time for a handoff", "\nhandofftimeout = \"0s\" " + theMarker + "\n", "handoff_timeout"},
	}
}

// impossibleSettings is one configuration file per setting outside the caps and
// the model aliases, each holding a value nothing could be done with.
func impossibleSettings() []badField {
	return []badField{
		{"a default model nobody defined", "\ndefaultmodel = \"nowhere\" " + theMarker + "\n", "default_model"},
		{"no default model at all", "\ndefaultmodel = \"\" " + theMarker + "\n", "default_model"},
		{"a fallback nobody defined", "\nfallbackchain = [\"nowhere\"] " + theMarker + "\n", "fallback_chain"},
		{"a phone number with no country code", "\nsignalaccount = \"5125550123\" " + theMarker + "\n", "signal_account"},
		{"a phone number with letters in it", "\nsignalaccount = \"+1512555ABCD\" " + theMarker + "\n", "signal_account"},
		{"a backup path that is not a full path", "\nbackuppath = \"backups\" " + theMarker + "\n", "backup_path"},
		{"a browser profile that is not a full path", "\nbrowserprofilepath = \"profile\" " + theMarker + "\n", "browser_profile_path"},
		{"a search server that is not an address", "\nsearchserveraddress = \"not an address\" " + theMarker + "\n", "search_server_address"},
		{"no model aliases at all", "\nmodels = [] " + theMarker + "\n", "models"},
		{"no sandbox roots at all", "\nsandboxroots = [] " + theMarker + "\n", "sandbox_roots"},
	}
}

// impossibleModelAliases is the good alias above with one line spoiled at a
// time, so that every rule about an alias is proved on its own.
func impossibleModelAliases() []badField {
	return []badField{
		{
			"an alias with no name",
			strings.Replace(oneGoodAlias, `name = "local"`, `name = "" `+theMarker, 1),
			"models.0.name",
		},
		{
			"an alias with a provider nobody has heard of",
			strings.Replace(oneGoodAlias, `provider = "openai"`, `provider = "carrier-pigeon" `+theMarker, 1),
			"models.0.provider",
		},
		{
			"an openai alias with no address",
			strings.Replace(oneGoodAlias, `base_address = "http://127.0.0.1:19091/v1"`, `base_address = "" `+theMarker, 1),
			"models.0.base_address",
		},
		{
			"an address that is not an address",
			strings.Replace(oneGoodAlias, `base_address = "http://127.0.0.1:19091/v1"`, `base_address = "127.0.0.1:19091" `+theMarker, 1),
			"models.0.base_address",
		},
		{
			"an alias that does not say which model",
			strings.Replace(oneGoodAlias, `model_name = "local-coder"`, `model_name = "" `+theMarker, 1),
			"models.0.model_name",
		},
		{
			"an alias that holds no tokens",
			strings.Replace(oneGoodAlias, `context_length = 262144`, `context_length = 0 `+theMarker, 1),
			"models.0.context_length",
		},
		{
			"a key written out instead of referred to",
			strings.Replace(oneGoodAlias, `context_length = 262144`, "context_length = 262144\nkeyreference = \"sk-a-real-key\" "+theMarker, 1),
			"models.0.key_reference",
		},
		{
			"a cli alias with no program to run",
			"\ndefaultmodel = \"cloud\"\n\n[[models]] " + theMarker + "\nname = \"cloud\"\nprovider = \"cli\"\nmodelname = \"opus\"\ncontextlength = 200000\n",
			"models.0.program",
		},
	}
}

func TestAnUnknownKeyIsRefusedByName(t *testing.T) {
	forEachKey := []struct {
		what     string
		document string
		key      string
	}{
		{"a key at the top level", "\nnosuchkey = 1 " + theMarker + "\n", "nosuchkey"},
		{"a key run together instead of written with underscores", "\ndefaultmodel = \"local\" " + theMarker + "\n", "defaultmodel"},
		{"a cap run together inside its table", "\n[caps]\nroundspertask = 100 " + theMarker + "\n", "caps.roundspertask"},
		{"an alias field run together", strings.Replace(oneGoodAlias, `model_name = "local-coder"`, "modelname = \"local-coder\" "+theMarker, 1), "models.modelname"},
		{"a misspelled key", "\ndefault_model = \"local\" " + theMarker + "\n", "default_model"},
		{"a key inside a table", "\n[caps]\nroundsperhour = 2 " + theMarker + "\n", "caps.rounds_per_hour"},
		{"a key inside a model alias", strings.Replace(oneGoodAlias, `name = "local"`, "name = \"local\"\nendpoint = \"x\" "+theMarker, 1), "models.endpoint"},
		{"the earliest of several", "\nfirstwrong = 1 " + theMarker + "\nsecondwrong = 2\n", "firstwrong"},
	}

	for _, key := range forEachKey {
		t.Run(key.what, func(t *testing.T) {
			problem := refuses(t, key.document)
			if problem.Key != key.key {
				t.Errorf("the problem names the key %q, want %q", problem.Key, key.key)
			}
			if want := markedLine(t, key.document); problem.Line != want {
				t.Errorf("the problem names line %d, want line %d", problem.Line, want)
			}
			if !strings.Contains(problem.Advice, "no such key") {
				t.Errorf("the advice is %q, want it to say the configuration has no such key", problem.Advice)
			}
		})
	}
}

func TestASandboxRootInsideTheAgentsOwnHomeFolderIsRefused(t *testing.T) {
	home := testkit.NewTempHome(t)
	inside := filepath.Join(home.Root, "work")

	problem := refusesRoot(t, home, inside)
	if !strings.Contains(problem.Advice, home.Root) {
		t.Errorf("the advice is %q, want it to name the home folder that must stay outside the sandbox", problem.Advice)
	}
}

func TestASandboxRootThatIsTheSSHFolderIsRefused(t *testing.T) {
	home := testkit.NewTempHome(t)
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("the test cannot find the user's home directory: %v", err)
	}

	problem := refusesRoot(t, home, filepath.Join(userHome, ".ssh"))
	if !strings.Contains(problem.Advice, ".ssh") {
		t.Errorf("the advice is %q, want it to name the SSH folder that must stay outside the sandbox", problem.Advice)
	}
}

func TestASandboxRootAboveTheUsersHomeDirectoryIsRefused(t *testing.T) {
	home := testkit.NewTempHome(t)
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("the test cannot find the user's home directory: %v", err)
	}

	problem := refusesRoot(t, home, filepath.Dir(userHome))
	if !strings.Contains(problem.Advice, "above your home directory") {
		t.Errorf("the advice is %q, want it to say the root is above the user's home directory", problem.Advice)
	}
}

func TestASandboxRootThatIsNotAFullPathIsRefused(t *testing.T) {
	home := testkit.NewTempHome(t)

	problem := refusesRoot(t, home, "work")
	if !strings.Contains(problem.Advice, "full path") {
		t.Errorf("the advice is %q, want it to ask for a full path", problem.Advice)
	}
}

func TestTheUsersOwnHomeDirectoryIsAGoodSandboxRoot(t *testing.T) {
	home := testkit.NewTempHome(t)
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("the test cannot find the user's home directory: %v", err)
	}
	if err := os.WriteFile(home.ConfigFile(), []byte("sandbox_roots = [\""+userHome+"\"]\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the configuration file for the test: %v", err)
	}

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("the user's home directory was refused as a sandbox root, and it is the shipped default: %v", err)
	}
	if len(settings.SandboxRoots) != 1 || settings.SandboxRoots[0] != userHome {
		t.Errorf("the sandbox roots are %v, want just the user's home directory", settings.SandboxRoots)
	}
}

// refusesRoot writes a configuration whose only sandbox root is the path, and
// returns the problem it produces.
func refusesRoot(t testing.TB, home contract.Home, root string) config.Problem {
	t.Helper()
	document := "sandbox_roots = [\"" + root + "\"]\n"
	if err := os.WriteFile(home.ConfigFile(), []byte(document), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the configuration file for the test: %v", err)
	}
	_, err := config.Load(home)
	if err == nil {
		t.Fatalf("the sandbox root %q was accepted, and it should not have been", root)
	}
	var problem config.Problem
	if !errors.As(err, &problem) {
		t.Fatalf("the error is %v of type %T, want a config.Problem naming the key", err, err)
	}
	if problem.Key != "sandbox_roots" {
		t.Errorf("the problem names the key %q, want sandbox_roots", problem.Key)
	}
	return problem
}

func TestAProblemPrintsAsPathLineKeyAndAdvice(t *testing.T) {
	withEverything := config.Problem{Path: "/home/someone/.coeus/config.toml", Line: 12, Key: "caps.rounds_per_task", Advice: "set it above zero"}
	if want := "/home/someone/.coeus/config.toml:12: caps.rounds_per_task: set it above zero"; withEverything.Error() != want {
		t.Errorf("the problem prints as %q, want %q", withEverything.Error(), want)
	}

	withNoLine := config.Problem{Path: "/home/someone/.coeus/config.toml", Key: "models", Advice: "add one"}
	if want := "/home/someone/.coeus/config.toml: models: add one"; withNoLine.Error() != want {
		t.Errorf("a problem with no line prints as %q, want %q", withNoLine.Error(), want)
	}

	aboutTheWholeFile := config.Problem{Path: "/home/someone/.coeus/config.toml", Advice: "this file is far too big"}
	if want := "/home/someone/.coeus/config.toml: this file is far too big"; aboutTheWholeFile.Error() != want {
		t.Errorf("a problem about the whole file prints as %q, want %q", aboutTheWholeFile.Error(), want)
	}
}
