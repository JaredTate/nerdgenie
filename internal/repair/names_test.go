package repair_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/repair"
)

// namedSpecs builds a set of pretend tools from names alone, for the tests that
// care only about which name a written name is read as.
func namedSpecs(names ...string) []contract.ToolSpec {
	specs := make([]contract.ToolSpec, 0, len(names))
	for _, name := range names {
		specs = append(specs, contract.ToolSpec{Name: name, Description: "A tool."})
	}
	return specs
}

// writtenCallReply is a reply holding one tool call written in the text form the
// harness asks for.
func writtenCallReply(name string, arguments string) contract.Reply {
	return contract.Reply{Text: contract.ToolCallOpenTag +
		`{"name": "` + name + `", "arguments": ` + arguments + "}" +
		contract.ToolCallCloseTag}
}

func TestNameRepairTriesTheFourRulesInOrder(t *testing.T) {
	cases := []struct {
		about   string
		written string
		specs   []contract.ToolSpec
		want    string
	}{
		{about: "the same name", written: "read", specs: testSpecs(), want: "read"},
		{about: "the same name in another case", written: "READ", specs: testSpecs(), want: "read"},
		{about: "the same name with a hyphen for the underscore", written: "Browser-Open", specs: testSpecs(), want: "browser_open"},
		{about: "the same name with the underscore left out", written: "browserOpen", specs: testSpecs(), want: "browser_open"},
		{about: "one edit away", written: "serch", specs: testSpecs(), want: "search"},
		{about: "two edits away", written: "brwser_opn", specs: testSpecs(), want: "browser_open"},
		{about: "the same name wins over the same name in another case",
			written: "read", specs: namedSpecs("Read", "read"), want: "read"},
		{about: "the same name in another case wins over the name without its underscore",
			written: "BROWSER_OPEN", specs: namedSpecs("browser_open", "browseropen"), want: "browser_open"},
		{about: "the name without its underscore wins over a name one edit away",
			written: "browser-open", specs: namedSpecs("browser_open", "browsers_open"), want: "browser_open"},
		{about: "the closer of two names within two edits",
			written: "browser_oper", specs: namedSpecs("browser_open", "browser_read"), want: "browser_open"},
	}

	for _, oneCase := range cases {
		t.Run(oneCase.about, func(t *testing.T) {
			result := repair.Find(writtenCallReply(oneCase.written, "{}"), oneCase.specs, 0)
			if result.Problem != "" {
				t.Fatalf("%q was refused with %q, want it read as %q", oneCase.written, result.Problem, oneCase.want)
			}
			if len(result.Calls) != 1 {
				t.Fatalf("%q produced %d calls, want one", oneCase.written, len(result.Calls))
			}
			if result.Calls[0].Name != oneCase.want {
				t.Errorf("%q was read as %q, want %q", oneCase.written, result.Calls[0].Name, oneCase.want)
			}
		})
	}
}

func TestARepairedNameIsReportedAndAnExactNameIsNot(t *testing.T) {
	repaired := repair.Find(writtenCallReply("READ", "{}"), testSpecs(), 0)
	if len(repaired.Repairs) != 1 {
		t.Fatalf("a repaired name reported %d repairs, want one", len(repaired.Repairs))
	}
	if repaired.Repairs[0].ModelWrote != "READ" || repaired.Repairs[0].RealName != "read" {
		t.Errorf("the repair says %+v, want READ read as read", repaired.Repairs[0])
	}

	exact := repair.Find(writtenCallReply("read", "{}"), testSpecs(), 0)
	if len(exact.Repairs) != 0 {
		t.Errorf("a name that was already right reported %d repairs, want none", len(exact.Repairs))
	}
}

func TestANameCloseToTwoRealToolsIsRefusedRatherThanGuessedAt(t *testing.T) {
	result := repair.Find(writtenCallReply("header", "{}"), namedSpecs("reader", "leader"), 0)

	if len(result.Calls) != 0 {
		t.Fatalf("a name close to two real tools produced %d calls, want none", len(result.Calls))
	}
	for _, wanted := range []string{"header", "reader", "leader", "equally close"} {
		if !strings.Contains(result.Problem, wanted) {
			t.Errorf("the problem %q does not mention %q", result.Problem, wanted)
		}
	}
}

func TestANameTooShortForTheEditRuleIsRefused(t *testing.T) {
	result := repair.Find(writtenCallReply("reed", "{}"), testSpecs(), 0)

	if len(result.Calls) != 0 {
		t.Fatalf("the four-letter name \"reed\" produced %d calls, want none", len(result.Calls))
	}
	if !strings.Contains(result.Problem, "reed") {
		t.Errorf("the problem %q does not name what the model wrote", result.Problem)
	}
}

func TestAnUnknownNameIsRefusedWithTheListOfRealTools(t *testing.T) {
	result := repair.Find(writtenCallReply("frobnicate", "{}"), testSpecs(), 0)

	if len(result.Calls) != 0 {
		t.Fatalf("an invented tool name produced %d calls, want none", len(result.Calls))
	}
	for _, spec := range testSpecs() {
		if !strings.Contains(result.Problem, spec.Name) {
			t.Errorf("the problem %q does not name the real tool %q", result.Problem, spec.Name)
		}
	}
}

func TestANameThatIsEmptyOrTooLongIsRefused(t *testing.T) {
	cases := map[string]string{
		"empty":               "",
		"only space":          "   ",
		"longer than the cap": strings.Repeat("r", 200),
	}

	for about, name := range cases {
		t.Run(about, func(t *testing.T) {
			reply := contract.Reply{ToolCalls: []contract.ToolCall{{ID: "call_1", Name: name, Input: []byte("{}")}}}
			result := repair.Find(reply, testSpecs(), 0)
			if len(result.Calls) != 0 {
				t.Fatalf("the name %q produced %d calls, want none", name, len(result.Calls))
			}
			if result.Problem == "" {
				t.Error("the name was refused with no problem for the model to read")
			}
		})
	}
}

func TestWithNoRealToolsEveryNameIsRefusedAndTheProblemSaysSo(t *testing.T) {
	result := repair.Find(writtenCallReply("read", "{}"), nil, 0)

	if len(result.Calls) != 0 {
		t.Fatalf("a call with no real tools produced %d calls, want none", len(result.Calls))
	}
	if !strings.Contains(result.Problem, "none") {
		t.Errorf("the problem %q does not say there are no tools", result.Problem)
	}
}
