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

// A valid permission rule, which the table below perturbs one line at a time.
const oneGoodRule = `
[[permission_rules]]
tool = "shell"
pattern = "git push*"
action = "ask"
`

func TestEveryFieldSetToTheWrongTypeIsRefusedWithItsKeyAndItsLine(t *testing.T) {
	eachIsRefused(t, fieldsOfTheWrongType())
}

// fieldsOfTheWrongType is one configuration file per field, each giving that
// field a value of a type it cannot hold.
func fieldsOfTheWrongType() []badField {
	return []badField{
		{"the default model as a number", "\ndefault_model = 7 " + theMarker + "\n", "default_model"},
		{"the fallback chain as one string", "\nfallback_chain = \"cloud\" " + theMarker + "\n", "fallback_chain"},
		{"the Signal account as a number", "\nsignal_account = 15125550123 " + theMarker + "\n", "signal_account"},
		{"the browser profile as a list", "\nbrowser_profile_path = [\"a\"] " + theMarker + "\n", "browser_profile_path"},
		{"the sandbox roots as a number", "\nsandbox_roots = 3 " + theMarker + "\n", "sandbox_roots"},
		{"the backup path as a boolean", "\nbackup_path = true " + theMarker + "\n", "backup_path"},
		{"the search server as a number", "\nsearch_server_address = 9 " + theMarker + "\n", "search_server_address"},
		{"the handoff timeout as nonsense", "\nhandoff_timeout = \"ninety\" " + theMarker + "\n", "handoff_timeout"},
		{"a round count as a word", "\n[caps]\nrounds_per_task = \"many\" " + theMarker + "\n", "caps.rounds_per_task"},
		{"a length of time as a list", "\n[caps]\ntime_per_task = [1] " + theMarker + "\n", "caps.time_per_task"},
		{"a memory cap as a boolean", "\n[memory_caps]\nuser_facts_bytes = true " + theMarker + "\n", "memory_caps.user_facts_bytes"},
		{"a context length with a fraction", "\n[[models]]\ncontext_length = 1.5 " + theMarker + "\n", "models.context_length"},
		{"the ask-me-first list as one string", "\nask_me_first = \"spending money\" " + theMarker + "\n", "ask_me_first"},
		{"the permission rules as a number", "\npermission_rules = 3 " + theMarker + "\n", "permission_rules"},
		{"a rule's action as a number", "\n[[permission_rules]]\naction = 7 " + theMarker + "\n", "permission_rules.action"},
	}
}

func TestEveryFieldSetToAnImpossibleValueIsRefusedWithItsKeyAndItsLine(t *testing.T) {
	eachIsRefused(t, impossibleCaps())
	eachIsRefused(t, impossibleSettings())
	eachIsRefused(t, impossibleModelAliases())
	eachIsRefused(t, impossiblePermissionRules())
}

// impossibleCaps is one configuration file per cap, each setting a limit that
// leaves the agent no room to work at all.
func impossibleCaps() []badField {
	return []badField{
		{"rounds in a task below zero", "\n[caps]\nrounds_per_task = -1 " + theMarker + "\n", "caps.rounds_per_task"},
		{"no queued messages", "\n[caps]\nqueued_messages = -1 " + theMarker + "\n", "caps.queued_messages"},
		{"no room in a tool result", "\n[caps]\ntool_output_bytes = 0 " + theMarker + "\n", "caps.tool_output_bytes"},
		{"no window for repeated calls", "\n[caps]\nidentical_call_window = 0 " + theMarker + "\n", "caps.identical_call_window"},
		{"time running backwards for a task", "\n[caps]\ntime_per_task = \"-1h\" " + theMarker + "\n", "caps.time_per_task"},
		{"no time for a tool", "\n[caps]\ntime_per_tool = \"0s\" " + theMarker + "\n", "caps.time_per_tool"},
		{"time running backwards for a tool", "\n[caps]\ntime_per_tool = \"-1m\" " + theMarker + "\n", "caps.time_per_tool"},
		{"time running backwards for a turn", "\n[caps]\ntime_per_turn = \"-1m\" " + theMarker + "\n", "caps.time_per_turn"},
		{"no room for world facts", "\n[memory_caps]\nworld_facts_bytes = 0 " + theMarker + "\n", "memory_caps.world_facts_bytes"},
		{"no room for user facts", "\n[memory_caps]\nuser_facts_bytes = -8 " + theMarker + "\n", "memory_caps.user_facts_bytes"},
		{"no time for a handoff", "\nhandoff_timeout = \"0s\" " + theMarker + "\n", "handoff_timeout"},
	}
}

// impossibleSettings is one configuration file per setting outside the caps and
// the model aliases, each holding a value nothing could be done with.
func impossibleSettings() []badField {
	return []badField{
		{"a default model nobody defined", "\ndefault_model = \"nowhere\" " + theMarker + "\n", "default_model"},
		{"no default model at all", "\ndefault_model = \"\" " + theMarker + "\n", "default_model"},
		{"a fallback nobody defined", "\nfallback_chain = [\"nowhere\"] " + theMarker + "\n", "fallback_chain"},
		{"a phone number with no country code", "\nsignal_account = \"5125550123\" " + theMarker + "\n", "signal_account"},
		{"a phone number with letters in it", "\nsignal_account = \"+1512555ABCD\" " + theMarker + "\n", "signal_account"},
		{"a backup path that is not a full path", "\nbackup_path = \"backups\" " + theMarker + "\n", "backup_path"},
		{"a browser profile that is not a full path", "\nbrowser_profile_path = \"profile\" " + theMarker + "\n", "browser_profile_path"},
		{"a search server that is not an address", "\nsearch_server_address = \"not an address\" " + theMarker + "\n", "search_server_address"},
		{"no model aliases at all", "\nmodels = [] " + theMarker + "\n", "models"},
		{"no sandbox roots at all", "\nsandbox_roots = [] " + theMarker + "\n", "sandbox_roots"},
		{"an ask-me-first entry nobody ships", "\nask_me_first = [\"anything at all\"] " + theMarker + "\n", "ask_me_first"},
		{"a sandbox setting that is neither of the two values", "\nsandbox = \"loose\" " + theMarker + "\n", "sandbox"},
	}
}

// impossiblePermissionRules is the user's own rulebook with one line spoiled at
// a time, so that every rule about a rule is proved on its own.
func impossiblePermissionRules() []badField {
	return []badField{
		{
			"a rule for no tool in particular",
			strings.Replace(oneGoodRule, `tool = "shell"`, `tool = "" `+theMarker, 1),
			"permission_rules.0.tool",
		},
		{
			"a rule with nothing to match on",
			strings.Replace(oneGoodRule, `pattern = "git push*"`, `pattern = "" `+theMarker, 1),
			"permission_rules.0.pattern",
		},
		{
			"a rule that does not say what to do",
			strings.Replace(oneGoodRule, `action = "ask"`, `action = "" `+theMarker, 1),
			"permission_rules.0.action",
		},
		{
			"a rule asking for something the rulebook cannot do",
			strings.Replace(oneGoodRule, `action = "ask"`, `action = "think about it" `+theMarker, 1),
			"permission_rules.0.action",
		},
		{
			"a rule that stops an unattended run, which the harness decides and the user does not write",
			strings.Replace(oneGoodRule, `action = "ask"`, `action = "stop" `+theMarker, 1),
			"permission_rules.0.action",
		},
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
			strings.Replace(oneGoodAlias, `context_length = 262144`, "context_length = 262144\nkey_reference = \"sk-a-real-key\" "+theMarker, 1),
			"models.0.key_reference",
		},
		{
			"a cli alias with no program to run",
			"\ndefault_model = \"cloud\"\n\n[[models]] " + theMarker + "\nname = \"cloud\"\nprovider = \"cli\"\nmodel_name = \"opus\"\ncontext_length = 200000\n",
			"models.0.program",
		},
		{
			"an alias asked to think at a level nobody offers",
			strings.Replace(oneGoodAlias, `context_length = 262144`, "context_length = 262144\nthink = \"hardest\" "+theMarker, 1),
			"models.0.think",
		},
	}
}

// TestAThinkLevelIsReadOffAnAliasAndAnUnknownOneNamesTheGoodOnes holds both
// halves of the think setting in config.toml: a level a provider knows is read
// onto the alias, and one nobody offers is refused with the six good ones in
// the message, because a person who wrote "hardest" needs to be told what to
// write instead.
func TestAThinkLevelIsReadOffAnAliasAndAnUnknownOneNamesTheGoodOnes(t *testing.T) {
	for _, level := range append(contract.ThinkLevels(), contract.ThinkDefault) {
		document := strings.Replace(oneGoodAlias, `context_length = 262144`,
			"context_length = 262144\nthink = \""+string(level)+"\"", 1)
		settings, err := config.Load(writeConfig(t, document))
		if err != nil {
			t.Fatalf("the think level %q was refused, and it is one Coeus offers: %v", level, err)
		}
		if settings.Models[0].Think != level {
			t.Errorf("the alias thinks at %q, want %q", settings.Models[0].Think, level)
		}
	}

	problem := refuses(t, strings.Replace(oneGoodAlias, `context_length = 262144`,
		"context_length = 262144\nthink = \"hardest\"", 1))
	for _, level := range contract.ThinkLevels() {
		if !strings.Contains(problem.Advice, string(level)) {
			t.Errorf("the advice is %q, and it leaves out the level %q a person could write instead", problem.Advice, level)
		}
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
		{"a misspelled key", "\ndefualt_model = \"local\" " + theMarker + "\n", "defualt_model"},
		{"a key inside a table", "\n[caps]\nrounds_per_hour = 2 " + theMarker + "\n", "caps.rounds_per_hour"},
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

func TestTheUsersOwnHomeDirectoryIsRefusedAsASandboxRootAndTheWorkFolderIsTheDefault(t *testing.T) {
	home := testkit.NewTempHome(t)
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("the test cannot find the user's home directory: %v", err)
	}
	problem := refusesRoot(t, home, userHome)
	if !strings.Contains(problem.Advice, "must stay outside the sandbox") {
		t.Errorf("the advice is %q, want it to say the home directory holds a path that must stay outside the sandbox", problem.Advice)
	}
	if err := os.WriteFile(home.ConfigFile(), []byte(""), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the configuration file for the test: %v", err)
	}
	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("an empty configuration was refused: %v", err)
	}
	if want := contract.DefaultSandboxRoots(userHome); len(settings.SandboxRoots) != 1 || settings.SandboxRoots[0] != want[0] {
		t.Errorf("the default sandbox roots are %v, want the work folder %v", settings.SandboxRoots, want)
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

func TestAnEmptyAskMeFirstListIsAllowed(t *testing.T) {
	home := writeConfig(t, "ask_me_first = []\n")

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("emptying the ask-me-first list was refused, and the design says the user may empty it: %v", err)
	}
	if len(settings.AskMeFirst) != 0 {
		t.Errorf("the ask-me-first list is %v, want it empty because the file emptied it", settings.AskMeFirst)
	}
}

func TestTheUsersOwnPermissionRulesLoad(t *testing.T) {
	home := writeConfig(t, oneGoodRule+"\n[[permission_rules]]\ntool = \"*\"\npattern = \"*\"\naction = \"allow\"\n")

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("loading two permission rules failed: %v", err)
	}
	if len(settings.PermissionRules) != 2 {
		t.Fatalf("the file writes two rules and %d came back", len(settings.PermissionRules))
	}
	first := settings.PermissionRules[0]
	if first.Tool != "shell" || first.Pattern != "git push*" || first.Action != contract.RulingAsk {
		t.Errorf("the first rule is %+v, want the shell rule the file writes", first)
	}
}

// TestBothSandboxValuesLoadAndAFileThatSaysNothingMeansOff pins the two values
// the sandbox setting takes and the default a file that says nothing gets, which
// is off: commands run straight on the machine as the user unless the file asks
// for the fence.
func TestBothSandboxValuesLoadAndAFileThatSaysNothingMeansOff(t *testing.T) {
	for _, written := range []struct {
		document string
		wanted   string
	}{
		{"", contract.SandboxOff},
		{"sandbox = \"off\"\n", contract.SandboxOff},
		{"sandbox = \"fence\"\n", contract.SandboxFence},
	} {
		settings, err := config.Load(writeConfig(t, written.document))
		if err != nil {
			t.Fatalf("the configuration %q was refused: %v", written.document, err)
		}
		if settings.SandboxMode() != written.wanted {
			t.Errorf("the configuration %q runs the sandbox as %q, want %q",
				written.document, settings.SandboxMode(), written.wanted)
		}
	}
}

// TestASandboxValueNobodyKnowsIsRefusedByNameWithBothValues holds the other half
// of the rule: a misspelled value is refused rather than quietly read as one of
// the two, and the refusal names both, so that the person can put it right
// without opening a document.
func TestASandboxValueNobodyKnowsIsRefusedByNameWithBothValues(t *testing.T) {
	problem := refuses(t, "sandbox = \"loose\"\n")

	if problem.Key != "sandbox" {
		t.Errorf("the problem names the key %q, want %q", problem.Key, "sandbox")
	}
	for _, wanted := range []string{"loose", contract.SandboxFence, contract.SandboxOff} {
		if !strings.Contains(problem.Advice, wanted) {
			t.Errorf("the advice is %q and does not name %q", problem.Advice, wanted)
		}
	}
}
