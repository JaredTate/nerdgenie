package contract_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

func TestThereAreExactlyEighteenBuiltInToolsAndJobReplacedSchedule(t *testing.T) {
	names := contract.BuiltInToolNames()

	if len(names) != 18 {
		t.Errorf("there are %d built-in tool names, want the eighteen from design section 7", len(names))
	}
	if !slices.Contains(names, contract.ToolJob) {
		t.Errorf("the tool list %v is missing the job tool", names)
	}
	if slices.Contains(names, "schedule") {
		t.Errorf("the tool list %v still holds the schedule tool, which the job tool replaced", names)
	}
	if len(slices.Compact(slices.Sorted(slices.Values(names)))) != len(names) {
		t.Errorf("the tool list %v holds a repeated name", names)
	}
}

func TestDescriptionWordCountCountsWhatTheRegistryWillReject(t *testing.T) {
	tests := []struct {
		description string
		want        int
	}{
		{"", 0},
		{"   ", 0},
		{"reads a file", 3},
		{"  reads   a  file  ", 3},
		{"reads a file,\na directory listing,\nor a past result", 10},
	}
	for _, test := range tests {
		if got := contract.DescriptionWordCount(test.description); got != test.want {
			t.Errorf("DescriptionWordCount(%q) is %d, want %d", test.description, got, test.want)
		}
	}
	if contract.MaxToolDescriptionWords != 40 {
		t.Errorf("the description cap is %d words, want 40", contract.MaxToolDescriptionWords)
	}
}

func TestDecodeUserToolSpecAcceptsAWellFormedDescription(t *testing.T) {
	described := `{
		"name": "wordcount",
		"description": "Counts the words in the text it is given. Use it when a length matters.",
		"fields": [{"name": "text", "type": "string", "description": "The text to count.", "required": true}],
		"classes": ["R"]
	}`

	spec, err := contract.DecodeUserToolSpec([]byte(described))
	if err != nil {
		t.Fatalf("decoding a well-formed description failed: %v", err)
	}
	if spec.Name != "wordcount" {
		t.Errorf("the tool is named %q, want wordcount", spec.Name)
	}
	if len(spec.Fields) != 1 || spec.Fields[0].Name != "text" || !spec.Fields[0].Required {
		t.Errorf("the input fields came back as %+v, want one required field named text", spec.Fields)
	}
	if len(spec.Classes) != 1 || spec.Classes[0] != contract.ClassRead {
		t.Errorf("the permission classes came back as %v, want the read class", spec.Classes)
	}
}

func TestDecodeUserToolSpecRefusesADescriptionTheRegistryCannotUse(t *testing.T) {
	tests := []struct {
		name     string
		describe string
	}{
		{"not json at all", "wordcount"},
		{"json that is not an object", "[]"},
		{"no name", `{"description":"Counts the words in the text."}`},
		{"a name with a space in it", `{"name":"word count","description":"Counts the words in the text."}`},
		{"no description", `{"name":"wordcount"}`},
		{"a description over forty words", `{"name":"wordcount","description":"` + strings.Repeat("word ", 41) + `"}`},
		{"an unknown permission class", `{"name":"wordcount","description":"Counts the words.","classes":["Q"]}`},
		{"a field with no name", `{"name":"wordcount","description":"Counts the words.","fields":[{"type":"string"}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := contract.DecodeUserToolSpec([]byte(test.describe)); err == nil {
				t.Fatalf("the description %s was accepted, want an error saying what is wrong", test.describe)
			}
		})
	}
}

func TestTheUserToolProtocolConstantsAreWhatTheDocumentSays(t *testing.T) {
	if contract.UserToolDescribeFlag != "--describe" {
		t.Errorf("a user tool is asked to describe itself with %q, want --describe", contract.UserToolDescribeFlag)
	}
	if contract.SecretReferencePrefix != "secret://" {
		t.Errorf("a secret is referred to with the prefix %q, want secret://", contract.SecretReferencePrefix)
	}
	if contract.RedactedMarker != "[redacted]" {
		t.Errorf("a redacted secret reads %q, want [redacted]", contract.RedactedMarker)
	}
}

func TestSecretReferenceNameReadsTheNameOutOfAReference(t *testing.T) {
	tests := []struct {
		reference string
		name      string
		valid     bool
	}{
		{"secret://x-account", "x-account", true},
		{"secret://a", "a", true},
		{"secret://", "", false},
		{"x-account", "", false},
		{"", "", false},
		{"secret:/x", "", false},
		{"secret://a b", "", false},
	}
	for _, test := range tests {
		name, valid := contract.SecretReferenceName(test.reference)
		if valid != test.valid {
			t.Errorf("SecretReferenceName(%q) said valid is %v, want %v", test.reference, valid, test.valid)
			continue
		}
		if valid && name != test.name {
			t.Errorf("SecretReferenceName(%q) read %q, want %q", test.reference, name, test.name)
		}
	}
}

func FuzzDecodeUserToolSpec(f *testing.F) {
	seeds := []string{
		`{"name":"wordcount","description":"Counts the words."}`,
		`{}`,
		``,
		`{"name":"a","description":"b","fields":[{}]}`,
	}
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, described []byte) {
		spec, err := contract.DecodeUserToolSpec(described)
		if err != nil {
			return
		}
		if spec.Name == "" {
			t.Fatal("a tool description with no name was accepted")
		}
		if contract.DescriptionWordCount(spec.Description) > contract.MaxToolDescriptionWords {
			t.Fatalf("a description of %d words was accepted, and the cap is %d", contract.DescriptionWordCount(spec.Description), contract.MaxToolDescriptionWords)
		}
		if _, err := json.Marshal(spec); err != nil {
			t.Fatalf("the accepted specification cannot be written back as JSON: %v", err)
		}
	})
}
