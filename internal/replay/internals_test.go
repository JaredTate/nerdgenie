package replay

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	workingcontext "github.com/JaredTate/nerdgenie/internal/context"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theStartOfTheseTests is where the fake clock of an internal test starts.
var theStartOfTheseTests = time.Date(2026, time.January, 10, 9, 0, 0, 0, time.UTC)

// aWorkingContextForTheseTests builds the real working context over a temporary
// home, which is the builder the running program hands a replay.
func aWorkingContextForTheseTests(t *testing.T) loop.ContextBuilder {
	t.Helper()
	built, err := workingcontext.New(workingcontext.Options{
		Home:            testkit.NewTempHome(t),
		MemoryCaps:      contract.DefaultConfig().MemoryCaps,
		MaxOutputTokens: contract.DefaultConfig().Caps.OutputTokensPerCall,
	})
	if err != nil {
		t.Fatalf("cannot build the working context over a temporary home: %v", err)
	}
	return loop.TheWorkingContext(built)
}

func TestTheQuietChannelSaysNothingAndApprovesEverything(t *testing.T) {
	ctx := context.Background()
	quiet := quietChannel{name: "terminal"}

	if quiet.Name() != "terminal" {
		t.Errorf("the quiet channel calls itself %q and it was built as the terminal", quiet.Name())
	}
	stream, err := quiet.Receive(ctx)
	if err != nil {
		t.Fatalf("the quiet channel refused to hand back a stream: %v", err)
	}
	if _, open := <-stream; open {
		t.Error("the quiet channel handed back a message, and a replay takes messages from nobody")
	}
	if err := quiet.Send(ctx, "anything at all"); err != nil {
		t.Errorf("the quiet channel refused a reply: %v", err)
	}
	if err := quiet.SendFile(ctx, "/tmp/nothing", "a caption"); err != nil {
		t.Errorf("the quiet channel refused a file: %v", err)
	}
	answer, err := quiet.ShowPreview(ctx, contract.Preview{ID: "1", Title: "a command", Body: "ls"})
	if err != nil || answer.Answer != contract.AnswerOnce {
		t.Errorf("the quiet channel answered a preview with %q and %v", answer.Answer, err)
	}
	if _, err := quiet.AskSecret(ctx, "the password"); !errors.Is(err, contract.ErrNoMaskedPrompt) {
		t.Errorf("the quiet channel answered a secret prompt with %v, and it must always refuse", err)
	}
	if health := quiet.Health(ctx); !health.Healthy {
		t.Errorf("the quiet channel says it is unwell: %s", health.Detail)
	}
}

func TestTheRecordedToolsNoticeACallTheRecordingNeverMade(t *testing.T) {
	ctx := context.Background()
	tools := newRecordedTools(Recording{TaskID: "3", Rounds: []Round{{Calls: []Call{
		{Name: "search", Result: "three files", Ran: true},
		{Name: contract.ToolTask, Result: "the record was written", Ran: true},
	}}}})

	if len(tools.Specs()) != 1 {
		t.Fatalf("the registry offers %+v and the record-writing tool is not one of its own", tools.Specs())
	}
	one, found := tools.Lookup("search")
	if !found {
		t.Fatal("the registry does not hold the one tool the recording used")
	}
	if one.Spec().Name != "search" {
		t.Errorf("the tool calls itself %q and it is the search", one.Spec().Name)
	}
	if _, found := tools.Lookup("browser_open"); found {
		t.Error("the registry answered for a tool the recording never used")
	}

	output, err := one.Run(ctx, nil)
	if err != nil || output.Text != "three files" {
		t.Fatalf("the recorded search gave back %q and %v", output.Text, err)
	}
	if tools.firstDifference() != "" {
		t.Errorf("the replay followed the recording and it was said to differ: %s", tools.firstDifference())
	}
	if _, err := one.Run(ctx, nil); err == nil {
		t.Error("a second search was answered and the recording only made one")
	}
	if !strings.Contains(tools.firstDifference(), "nothing left to answer with") {
		t.Errorf("the difference is %q and the recording had run out", tools.firstDifference())
	}
}

func TestTheRecordedToolsNoticeADifferentToolAndKeepOnlyTheFirst(t *testing.T) {
	ctx := context.Background()
	tools := newRecordedTools(Recording{TaskID: "3", Rounds: []Round{{Calls: []Call{
		{Name: "search", Result: "three files", Ran: true},
		{Name: "read", Result: "the file itself", Ran: true},
	}}}})

	read, _ := tools.Lookup("read")
	if _, err := read.Run(ctx, nil); err != nil {
		t.Fatalf("the recorded read gave back %v", err)
	}
	first := tools.firstDifference()
	if !strings.Contains(first, `asked for "read"`) || !strings.Contains(first, `recording asked for "search"`) {
		t.Fatalf("the difference is %q and the replay read where the recording searched", first)
	}
	search, _ := tools.Lookup("search")
	if _, err := search.Run(ctx, nil); err != nil {
		t.Fatalf("the recorded search gave back %v", err)
	}
	if tools.firstDifference() != first {
		t.Errorf("the difference became %q and only the first one is kept", tools.firstDifference())
	}
}

func TestTheRecordedToolsNoticeACallThatWasNeverRepeated(t *testing.T) {
	tools := newRecordedTools(Recording{TaskID: "3", Rounds: []Round{{Calls: []Call{
		{Name: "search", Result: "three files", Ran: true},
	}}}})

	if !strings.Contains(tools.firstDifference(), "the replay never did") {
		t.Errorf("the difference is %q and the replay ran nothing at all", tools.firstDifference())
	}
}

func TestTheRecordedModelNamesTheTaskItIsPlayingBack(t *testing.T) {
	model := newRecordedModel(Recording{TaskID: "17", Answer: "it is done"})

	if !strings.Contains(model.Name(), "17") {
		t.Errorf("the recorded model calls itself %q and it plays task 17 back", model.Name())
	}
	if model.ContextLength() != RecordedContextLength {
		t.Errorf("the recorded model says its window is %d", model.ContextLength())
	}
	streamed := ""
	reply, err := model.Send(context.Background(), contract.Request{}, func(delta string) { streamed += delta })
	if err != nil {
		t.Fatalf("the recorded model refused a call: %v", err)
	}
	if reply.Text != "it is done" || reply.Finish != contract.FinishEnd {
		t.Errorf("the recorded model answered %q and finished %q", reply.Text, reply.Finish)
	}
	if streamed != "it is done" {
		t.Errorf("the recorded model streamed %q", streamed)
	}
}

func TestTheRecordedModelStopsWhenTheCallIsGivenUpOn(t *testing.T) {
	model := newRecordedModel(Recording{TaskID: "17"})
	ctx, giveUp := context.WithCancel(context.Background())
	giveUp()

	if _, err := model.Send(ctx, contract.Request{}, nil); err == nil {
		t.Fatal("the recorded model answered a call that had been given up on")
	}
}

func TestACallWithNoArgumentsIsGivenAnEmptyObject(t *testing.T) {
	if written := string(inputOf(Call{Name: "search"})); written != "{}" {
		t.Errorf("a call with no arguments was written as %q", written)
	}
	if written := string(inputOf(Call{Name: "search", Input: []byte(`{"pattern":"*"}`)})); written != `{"pattern":"*"}` {
		t.Errorf("a call's arguments were changed to %q", written)
	}
}

func TestTheRecordsAreComparedWithoutWhatARunCannotRepeat(t *testing.T) {
	was := aSmallRecord("17", 40)
	now := aSmallRecord("2", 7)

	if difference := differenceBetween(was, now); difference != "" {
		t.Errorf("two records that differ only in their number and budget were said to differ: %s", difference)
	}
	// A recording made under a budget replays on a machine with none, and
	// whether there was a budget is as much a fact of the run as how much of
	// it was left.
	none := aSmallRecord("2", 0)
	none.Header.NoRoundBudget, none.Header.NoTimeBudget = true, true
	if difference := differenceBetween(was, none); difference != "" {
		t.Errorf("a record with a budget and one with none were said to differ: %s", difference)
	}
	now.Goal.Why = "for some other reason"
	if difference := differenceBetween(was, now); !strings.Contains(difference, "some other reason") {
		t.Errorf("the difference is %q and the why is what changed", difference)
	}
	longer := aSmallRecord("2", 7)
	longer.Lessons.Failures = []contract.Failure{
		{ID: "F1", Text: "the first draft was too long", Cause: "three facts in one post"},
	}
	if difference := differenceBetween(was, longer); !strings.Contains(difference, "Failures:") {
		t.Errorf("the difference is %q and one record holds a failure the other does not", difference)
	}
}

func TestTheReportSaysWhatTheDoneCheckMadeOfEachRun(t *testing.T) {
	said := map[string]Result{
		"the done-check passes, the same as the recording":                  {},
		"the done-check says what it said on the recording: no done list":   {DoneCheck: "no done list", WasDoneCheck: "no done list"},
		"the done-check passes, and on the recording it said: no done list": {WasDoneCheck: "no done list"},
		"the done-check says no done list, and on the recording it passed":  {DoneCheck: "no done list"},
		"the done-check says one line is waiting, and on the recording it said no done list": {
			DoneCheck: "one line is waiting", WasDoneCheck: "no done list",
		},
	}
	for wanted, result := range said {
		if line := doneCheckLine(result); line != wanted {
			t.Errorf("the done-check line is %q and it should read %q", line, wanted)
		}
	}
}

func TestTheReportCountsOneRoundAsARound(t *testing.T) {
	if word := roundOrRounds(1); word != "round" {
		t.Errorf("one round is called %q", word)
	}
	if word := roundOrRounds(2); word != "rounds" {
		t.Errorf("two rounds are called %q", word)
	}
}

func TestAQuestionLeavesOutTheMarkOnAWithdrawnFact(t *testing.T) {
	asked := questionAbout(contract.Fact{Text: "(superseded by m12) the user keeps their notes in the notes folder"})
	if asked != "the user keeps their notes in the notes folder" {
		t.Errorf("the question is %q and the mark on the fact belongs to memory rather than to the question", asked)
	}
	long := strings.Repeat("a very long fact ", 40)
	if len([]rune(questionAbout(contract.Fact{Text: long}))) != MaxQuestionRunes {
		t.Errorf("a long fact was asked about in %d letters", len([]rune(questionAbout(contract.Fact{Text: long}))))
	}
	if asked := questionAbout(contract.Fact{Text: "   "}); asked != "" {
		t.Errorf("a fact with no words was asked about as %q", asked)
	}
}

func TestTheNamesOfWhatAQuestionBroughtBack(t *testing.T) {
	if named := idsOf(nil); named != "nothing" {
		t.Errorf("a question that found nothing is reported as %q", named)
	}
	if named := idsOf([]contract.Fact{{ID: "m1"}, {ID: "m2"}}); named != "m1, m2" {
		t.Errorf("the facts a question found are reported as %q", named)
	}
}

func TestTheOneLineNamesOnlyTheFirstFewFailures(t *testing.T) {
	report := Report{}
	checks := []Check{}
	for at := range MaxFailuresNamed + 3 {
		checks = append(checks, Check{Name: "check " + string(rune('a'+at))})
	}
	report.add(checks)
	report.Line = oneLine(report)

	if !strings.Contains(report.Line, "and 3 more") {
		t.Errorf("the one line is %q and three failures were left unnamed", report.Line)
	}
	if strings.Contains(report.Line, "check f") {
		t.Errorf("the one line is %q and it names more than %d failures", report.Line, MaxFailuresNamed)
	}
}

func TestATaskNumberThatIsNotANumberIsRefused(t *testing.T) {
	if _, err := nameOfTask(""); err == nil {
		t.Error("a recording with no task number was turned into a test anyway")
	}
	if _, err := nameOfTask("17"); err != nil {
		t.Errorf("the task number 17 was refused: %v", err)
	}
}

func TestReadingARecordingNeedsALogToReadFrom(t *testing.T) {
	if _, err := Read(context.Background(), nil, "17"); err == nil {
		t.Error("a recording was read out of no log at all")
	}
}

// aSmallRecord is the least a task record can hold and still print, which is
// what the comparison tests hold up against each other.
func aSmallRecord(taskID string, roundsLeft int) contract.Record {
	return contract.Record{
		Header: contract.Header{
			Kind: contract.RecordTask, ID: taskID, Status: contract.StatusDone,
			Origin: "terminal", RoundsLeft: roundsLeft, MinutesLeft: roundsLeft,
			Cost: contract.CostLine{InputTokens: roundsLeft * 100},
		},
		Goal: contract.Goal{Ask: "count the files", Why: "the user wants a count"},
	}
}
