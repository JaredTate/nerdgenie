package config

import (
	"errors"
	"strings"
	"testing"
)

func TestKeyLinesNotesEveryKeyWithTheTableItIsIn(t *testing.T) {
	document := strings.Join([]string{
		"# a comment on line one",
		"default_model = \"local\"",
		"memory_caps.user_facts_bytes = 4000",
		"",
		"[caps]",
		"rounds_per_task = 100 # and a comment after the value",
		"",
		"[[models]]",
		"name = \"local\"",
		"",
		"[[models]]",
		"name = \"cloud\"",
		"\"quoted key\" = 1",
	}, "\n")

	lines := keyLines(document)
	forEachKey := map[string]int{
		"default_model":                2,
		"memory_caps.user_facts_bytes": 3,
		"Memory_Caps.User_Facts_Bytes": 3,
		"caps":                         5,
		"caps.rounds_per_task":         6,
		"models":                       8,
		"models.0":                     8,
		"models.0.name":                9,
		"models.name":                  9,
		"models.1":                     11,
		"models.1.name":                12,
		"models.1.quoted key":          13,
		"caps.nosuchkey":               5,
		"nothing.like.this":            0,
		"models.0.name.deeper.still":   9,
	}
	for key, want := range forEachKey {
		if got := lines.of(key); got != want {
			t.Errorf("the key %q is noted on line %d, want line %d", key, got, want)
		}
	}
}

func TestKeyLinesIgnoresLinesThatAreNotKeys(t *testing.T) {
	document := strings.Join([]string{
		"[unfinished",
		"[[alsounfinished",
		"a line with no equals sign",
		"   ",
		"# just a comment",
		"[]",
		"[[]]",
	}, "\n")

	if lines := keyLines(document); len(lines) != 0 {
		t.Errorf("the locator noted %v, want nothing, because none of those lines names a key", lines)
	}
}

func TestKeyLinesKeepsTheFirstOfARepeatedKey(t *testing.T) {
	lines := keyLines("default_model = \"a\"\ndefault_model = \"b\"\n")
	if got := lines.of("default_model"); got != 1 {
		t.Errorf("a key written twice is noted on line %d, want the first one, line 1", got)
	}
}

func TestWithoutTheNumberOnlyTakesOffAnEntryNumber(t *testing.T) {
	forEachTable := map[string]string{
		"models.0":       "models",
		"models.12":      "models",
		"caps":           "caps",
		"caps.something": "caps.something",
		"":               "",
	}
	for table, want := range forEachTable {
		if got := withoutTheNumber(table); got != want {
			t.Errorf("the table %q without its entry number is %q, want %q", table, got, want)
		}
	}
}

func TestJoinKeyPutsTheTableInFront(t *testing.T) {
	forEachPair := []struct {
		table string
		key   string
		want  string
	}{
		{"caps", "rounds_per_task", "caps.rounds_per_task"},
		{"", "default_model", "default_model"},
		{"caps", "", "caps"},
		{"", "", ""},
	}
	for _, pair := range forEachPair {
		if got := joinKey(pair.table, pair.key); got != pair.want {
			t.Errorf("joining %q and %q gives %q, want %q", pair.table, pair.key, got, pair.want)
		}
	}
}

func TestHealthAddressSitsBesideTheApiRatherThanInsideIt(t *testing.T) {
	forEachAddress := map[string]string{
		"http://127.0.0.1:19091/v1":  "http://127.0.0.1:19091/health",
		"http://127.0.0.1:19091/v1/": "http://127.0.0.1:19091/health",
		"http://127.0.0.1:19091":     "http://127.0.0.1:19091/health",
		"http://127.0.0.1:19091/":    "http://127.0.0.1:19091/health",
		"https://host/api/v1":        "https://host/api/health",
		"  http://host/v1  ":         "http://host/health",
		"":                           "",
	}
	for address, want := range forEachAddress {
		if got := healthAddress(address); got != want {
			t.Errorf("the health check for %q is at %q, want %q", address, got, want)
		}
	}
}

func TestReadDecodeMessageTakesTheLineAndTheKeyOutOfWhatTheLibrarySays(t *testing.T) {
	forEachSentence := []struct {
		what     string
		sentence string
		line     int
		key      string
		message  string
	}{
		{
			"a value of the wrong type",
			`toml: line 4 (last key "caps.rounds_per_task"): incompatible types`,
			4, "caps.rounds_per_task", "incompatible types",
		},
		{
			"a sentence with no line at all",
			"toml: something went wrong somewhere",
			0, "", "something went wrong somewhere",
		},
		{"a line number that is not a number", `toml: line four (last key "x"): oh dear`, 0, "", `toml: line four (last key "x"): oh dear`},
		{"a sentence that stops early", "toml: line 4 and nothing more", 0, "", "toml: line 4 and nothing more"},
		{"a key that is never closed", `toml: line 4 (last key "x"`, 4, "", `toml: line 4 (last key "x"`},
	}
	for _, sentence := range forEachSentence {
		t.Run(sentence.what, func(t *testing.T) {
			line, key, message := readDecodeMessage(sentence.sentence)
			if line != sentence.line || key != sentence.key || message != sentence.message {
				t.Errorf("reading %q gives line %d, key %q, message %q; want line %d, key %q, message %q",
					sentence.sentence, line, key, message, sentence.line, sentence.key, sentence.message)
			}
		})
	}
}

func TestProblemFromDecodeErrorAlwaysSaysWhatToDo(t *testing.T) {
	problem := problemFromDecodeError("/home/someone/.nerdgenie/config.toml", errors.New("toml: nothing useful was said here"))
	if !strings.Contains(problem.Advice, "so give the key a value") {
		t.Errorf("the advice is %q, want it to say what to do even when the library said little", problem.Advice)
	}
}

func TestFolderHoldsAnswersForTheRootAndForItself(t *testing.T) {
	forEachPair := []struct {
		folder string
		path   string
		want   bool
	}{
		{"/", "/home/someone", true},
		{"/home", "/home/someone", true},
		{"/home/", "/home/someone", true},
		{"/home/someone", "/home/someone", true},
		{"/home/someone", "/home/someone-else", false},
		{"/home/someone/work", "/home/someone", false},
	}
	for _, pair := range forEachPair {
		if got := folderHolds(pair.folder, pair.path); got != pair.want {
			t.Errorf("asking whether %q holds %q gives %v, want %v", pair.folder, pair.path, got, pair.want)
		}
	}
}

func TestLooksLikeAPhoneNumberAcceptsInternationalFormAndNothingElse(t *testing.T) {
	forEachAccount := map[string]bool{
		"+15125550123":      true,
		"+441234567":        true,
		"+123456789012345":  true,
		"+1234567":          true,
		"+123456":           false,
		"+1234567890123456": false,
		"+01234567":         false,
		"15125550123":       false,
		"+1512555 0123":     false,
		"+1512555ABCD":      false,
		"+":                 false,
		"":                  false,
	}
	for account, want := range forEachAccount {
		if got := looksLikeAPhoneNumber(account); got != want {
			t.Errorf("asking whether %q is a phone number gives %v, want %v", account, got, want)
		}
	}
}

func TestIsWebAddressWantsBothASchemeAndAHost(t *testing.T) {
	forEachAddress := map[string]bool{
		"http://127.0.0.1:19091/v1":  true,
		"https://search.example.com": true,
		"127.0.0.1:19091":            false,
		"/just/a/path":               false,
		"http://":                    false,
		"":                           false,
		"://nonsense":                false,
	}
	for address, want := range forEachAddress {
		if got := isWebAddress(address); got != want {
			t.Errorf("asking whether %q is a web address gives %v, want %v", address, got, want)
		}
	}
}
