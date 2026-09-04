package functional

import (
	"bytes"
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool"
)

// TestTheExampleUserToolAnswersThroughTheRealRegistry copies the worked example
// from examples/tools/wordcount into a temporary home's tools folder, builds the
// real registry, and lets the fake model call it the way the sample functional
// test calls anything else. It proves the four claims docs/EXTENDING.md makes
// about a user tool: the executable goes in the tools folder, the harness asks
// it what it is at startup, the model sees it beside the built-in tools, and
// calling it hands the model the text the program printed.
func TestTheExampleUserToolAnswersThroughTheRealRegistry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	home := testkit.NewTempHome(t)
	installUserTool(t, home, exampleToolSource(t))

	registry, _ := newRegistryForTest(ctx, t, home)
	if _, found := registry.Lookup("wordcount"); !found {
		t.Fatalf("the registry did not load wordcount from %s", home.ToolsFolder())
	}
	if !describedIn(registry.Specs(), "wordcount") {
		t.Error("the model is not told about wordcount, although the registry loaded it")
	}

	channel := testkit.NewFakeChannel("terminal")
	model := testkit.NewFakeModel(testkit.Script{
		Name:          "extending",
		ContextLength: 24000,
		Steps: []testkit.Step{{
			Expect: []string{"how many words"},
			ToolCalls: []contract.ToolCall{{
				ID:    "c1",
				Name:  "wordcount",
				Input: json.RawMessage(`{"text":"one two three four five"}`),
			}},
			Finish: contract.FinishToolCalls,
		}, {
			Expect: []string{"5"},
			Text:   "That is 5 words.",
			Finish: contract.FinishEnd,
		}},
	})

	inbound, err := channel.Receive(ctx)
	if err != nil {
		t.Fatalf("attaching to the channel failed: %v", err)
	}
	if err := channel.Push(contract.Inbound{ID: "1", Sender: "jared", Text: "how many words are in that line?"}); err != nil {
		t.Fatalf("pushing the message failed: %v", err)
	}

	if err := within(t, 20*time.Second, func() error {
		return answerWithTools(ctx, inbound, model, registry, channel)
	}); err != nil {
		t.Fatalf("answering the message failed: %v", err)
	}

	sent := channel.Sent()
	if len(sent) != 1 {
		t.Fatalf("the channel carried %d replies, want 1", len(sent))
	}
	if sent[0] != "That is 5 words." {
		t.Errorf("the reply was %q, want the count the example tool produced", sent[0])
	}
}

// TestTheGuidesFourContractChecksAreTheOnesTestkitHolds reads the four check
// functions docs/EXTENDING.md tells a contributor to run, one per extension
// point, and compares them with the check functions internal/testkit really
// exports. A check that is renamed or dropped makes the guide wrong, and this is
// what says so.
func TestTheGuidesFourContractChecksAreTheOnesTestkitHolds(t *testing.T) {
	root := repositoryRoot(t)

	named := checksNamedInTheGuide(t, root)
	if len(named) != 4 {
		t.Fatalf("the guide names %d contract checks (%s), want exactly one per extension point", len(named), strings.Join(named, ", "))
	}

	held := checksTestkitExports(t, root)
	for _, name := range named {
		if !held[name] {
			t.Errorf("docs/EXTENDING.md tells a contributor to run testkit.%s, which internal/testkit does not export", name)
		}
	}
}

// TestADescriptionOverTheCapIsRefusedWithTheGuidesMessage takes the worked
// example, lengthens its description past the forty-word cap, and shows two
// things: the program's own answer is refused with exactly the line the guide
// quotes, and the registry skips the tool rather than refusing to start.
func TestADescriptionOverTheCapIsRefusedWithTheGuidesMessage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	wanted := refusalQuotedInTheGuide(t, repositoryRoot(t))

	home := testkit.NewTempHome(t)
	installUserTool(t, home, withALongerDescription(t, exampleToolSource(t), 41))

	_, err := contract.DecodeUserToolSpec(describeItself(ctx, t, filepath.Join(home.ToolsFolder(), "wordcount")))
	if err == nil {
		t.Fatal("a description of 41 words was accepted, want the refusal the guide quotes")
	}
	if err.Error() != wanted {
		t.Errorf("the refusal was %q, want the line docs/EXTENDING.md quotes, %q", err.Error(), wanted)
	}

	registry, notes := newRegistryForTest(ctx, t, home)
	if _, found := registry.Lookup("wordcount"); found {
		t.Error("the registry loaded a tool whose description is over the cap")
	}
	if len(*notes) != 1 {
		t.Fatalf("the registry said %d things about what it skipped, want one line naming the file and the problem: %v", len(*notes), *notes)
	}
	if !strings.Contains((*notes)[0], wanted) {
		t.Errorf("the skipped line was %q, want it to carry the refusal %q", (*notes)[0], wanted)
	}
	if !strings.Contains((*notes)[0], home.ToolsFolder()) {
		t.Errorf("the skipped line was %q, want it to name the file it skipped", (*notes)[0])
	}
}

// answerWithTools is the smallest stand-in for the agent loop that can run a
// tool: ask the model, run whatever it asked for through the registry, hand the
// results back, and send its next answer to the user. It is the sample test's
// answerOneMessage with tool rounds added, and like every loop in Coeus it has a
// limit.
func answerWithTools(ctx context.Context, inbound <-chan contract.Inbound, model contract.Model, registry contract.ToolRegistry, channel contract.Channel) error {
	var message contract.Inbound
	select {
	case message = <-inbound:
	case <-ctx.Done():
		return ctx.Err()
	}

	request := contract.Request{
		Messages: []contract.Message{{Role: contract.RoleUser, Text: message.Text}},
		Tools:    registry.Specs(),
	}
	for round := 0; round < 4; round++ {
		reply, err := model.Send(ctx, request, nil)
		if err != nil {
			return err
		}
		if len(reply.ToolCalls) == 0 {
			return channel.Send(ctx, reply.Text)
		}
		request.Messages = append(request.Messages, contract.Message{Role: contract.RoleAssistant, ToolCalls: reply.ToolCalls})
		results, err := runTheCalls(ctx, registry, reply.ToolCalls)
		if err != nil {
			return err
		}
		request.Messages = append(request.Messages, contract.Message{Role: contract.RoleUser, ToolResults: results})
	}
	return errorString("the model asked for tools four times running, and this test allows no more")
}

// errorString is the smallest error a test file needs.
type errorString string

// Error says what went wrong.
func (text errorString) Error() string { return string(text) }

// runTheCalls runs every tool the model asked for and returns the results in the
// order they were asked for.
func runTheCalls(ctx context.Context, registry contract.ToolRegistry, calls []contract.ToolCall) ([]contract.ToolResult, error) {
	results := make([]contract.ToolResult, 0, len(calls))
	for _, call := range calls {
		found, ok := registry.Lookup(call.Name)
		if !ok {
			return nil, errorString("the model asked for a tool the registry does not hold: " + call.Name)
		}
		output, err := found.Run(ctx, call.Input)
		if err != nil {
			return nil, err
		}
		results = append(results, contract.ToolResult{CallID: call.ID, Text: output.Text})
	}
	return results, nil
}

// describedIn says whether the specifications the model is given hold this tool.
func describedIn(specs []contract.ToolSpec, name string) bool {
	for _, spec := range specs {
		if spec.Name == name {
			return true
		}
	}
	return false
}

// exampleToolSource is the text of the worked example this repository ships.
func exampleToolSource(t *testing.T) []byte {
	t.Helper()
	source, err := os.ReadFile(filepath.Join(repositoryRoot(t), "examples", "tools", "wordcount", "wordcount"))
	if err != nil {
		t.Fatalf("cannot read the worked example: %v", err)
	}
	return source
}

// descriptionLine finds the one line of the worked example that holds the
// description the tool reports, so that a test can lengthen it.
var descriptionLine = regexp.MustCompile(`(?m)^description=".*"$`)

// withALongerDescription returns the example with its description replaced by
// one of exactly this many words, which is how a test builds a tool the cap must
// refuse.
func withALongerDescription(t *testing.T, source []byte, words int) []byte {
	t.Helper()
	longer := strings.TrimSpace(strings.Repeat("word ", words))
	replaced := descriptionLine.ReplaceAll(source, []byte(`description="`+longer+`"`))
	if bytes.Equal(replaced, source) {
		t.Fatal("the worked example no longer holds a description= line, so this test cannot lengthen it")
	}
	return replaced
}

// describeItself runs a user tool the way the harness does at startup and gives
// back what it printed.
func describeItself(ctx context.Context, t *testing.T, path string) []byte {
	t.Helper()
	command := exec.CommandContext(ctx, path, contract.UserToolDescribeFlag)
	described, err := command.Output()
	if err != nil {
		t.Fatalf("running %s %s failed: %v", path, contract.UserToolDescribeFlag, err)
	}
	return described
}

// installUserTool writes one executable into the home's tools folder, which is
// the whole of installing a user tool.
func installUserTool(t *testing.T, home contract.Home, source []byte) {
	t.Helper()
	path := filepath.Join(home.ToolsFolder(), "wordcount")
	if err := os.WriteFile(path, source, 0o755); err != nil {
		t.Fatalf("cannot write the tool into %s: %v", home.ToolsFolder(), err)
	}
}

// newRegistryForTest builds the real tool registry over a temporary home, which
// is what loads the executables in the tools folder, and hands back the lines it
// said about anything it skipped.
func newRegistryForTest(ctx context.Context, t *testing.T, home contract.Home) (contract.ToolRegistry, *[]string) {
	t.Helper()
	notes := new([]string)
	registry, err := tool.New(ctx, tool.Settings{
		Home: home,
		Note: func(line string) { *notes = append(*notes, line) },
	})
	if err != nil {
		t.Fatalf("building the tool registry failed: %v", err)
	}
	return registry, notes
}

// guideText reads docs/EXTENDING.md, which is the document these tests hold to
// its word.
func guideText(t *testing.T, root string) string {
	t.Helper()
	text, err := os.ReadFile(filepath.Join(root, "docs", "EXTENDING.md"))
	if err != nil {
		t.Fatalf("cannot read docs/EXTENDING.md: %v", err)
	}
	return string(text)
}

// checkInTheGuide finds the name of a contract check wherever the guide names
// one, such as "testkit.CheckChannel".
var checkInTheGuide = regexp.MustCompile(`testkit\.(Check[A-Za-z]+)`)

// checksNamedInTheGuide is the sorted set of contract checks docs/EXTENDING.md
// tells a contributor to run.
func checksNamedInTheGuide(t *testing.T, root string) []string {
	t.Helper()
	seen := map[string]bool{}
	for _, found := range checkInTheGuide.FindAllStringSubmatch(guideText(t, root), -1) {
		seen[found[1]] = true
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// checksTestkitExports is the set of contract checks internal/testkit really
// holds, read from the source rather than from a list a test keeps of its own,
// because a list kept here would drift the same way the guide can.
func checksTestkitExports(t *testing.T, root string) map[string]bool {
	t.Helper()
	folder := filepath.Join(root, "internal", "testkit")
	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatalf("cannot read %s: %v", folder, err)
	}
	positions := token.NewFileSet()
	held := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		file, err := parser.ParseFile(positions, filepath.Join(folder, entry.Name()), nil, 0)
		if err != nil {
			t.Fatalf("cannot read %s: %v", entry.Name(), err)
		}
		for _, declared := range file.Decls {
			function, ok := declared.(*ast.FuncDecl)
			if ok && function.Recv == nil && strings.HasPrefix(function.Name.Name, "Check") {
				held[function.Name.Name] = true
			}
		}
	}
	if len(held) == 0 {
		t.Fatalf("no contract checks were found in %s, so this test proves nothing", folder)
	}
	return held
}

// refusalInTheGuide finds the refusal docs/EXTENDING.md quotes for a tool
// description over the forty-word cap, between the backticks it is written in.
var refusalInTheGuide = regexp.MustCompile("`(the description of [^`]+)`")

// refusalQuotedInTheGuide is that refusal, word for word.
func refusalQuotedInTheGuide(t *testing.T, root string) string {
	t.Helper()
	found := refusalInTheGuide.FindStringSubmatch(guideText(t, root))
	if found == nil {
		t.Fatal("docs/EXTENDING.md quotes no refusal for a description over the cap, so a contributor is not told what a refusal looks like")
	}
	return found[1]
}
