//go:build live

// The forty-step fixture through a real "coeus serve", one test per real model.
//
// The fixture is the proof that the record and the working context work on any
// model, and internal/context/live_test.go already plays it to its last round
// and sends the prompt built from it to each of the three. This is the same
// fixture one floor up: its forty rounds are played into the home folder's own
// event log before the agent starts, so that the record under test is the one a
// real agent reads back out of SQLite over the socket the terminal uses, and the
// model is asked for a real answer with that record sitting in the log beside it.
//
// The three assertions are the plan's: the ask and the correction are byte for
// byte what the user wrote, the done-check passes, and every result id is still
// readable. They are checked before the model runs and again afterwards, because
// a real model working in the same agent must not be able to disturb them.
package functional

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/log"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theFixtureQuestion is what the model is asked while the fixture's record sits
// in the log beside it. It is the same short question internal/context's live
// test asks, so that the two tiers cost the same and can be compared.
const theFixtureQuestion = "In one short sentence, say what a task record is for."

// The budget the fixture's seeded record is opened with, which is the shipped
// one: a hundred rounds and an hour.
const (
	fixtureRoundsLeft  = 100
	fixtureMinutesLeft = 60
)

func TestTheFortyStepFixtureHoldsOnTheLocalModelThroughTheRealServe(t *testing.T) {
	theFortyStepFixtureOnARealModel(t, theLocalModel())
}

func TestTheFortyStepFixtureHoldsOnOpusThroughTheRealServe(t *testing.T) {
	theFortyStepFixtureOnARealModel(t, theClaudeModel())
}

func TestTheFortyStepFixtureHoldsOnGPTThroughTheRealServe(t *testing.T) {
	theFortyStepFixtureOnARealModel(t, theCodexModel())
}

// theFortyStepFixtureOnARealModel is the body every one of the three tests runs.
func theFortyStepFixtureOnARealModel(t *testing.T, model liveModel) {
	t.Helper()
	fixture, err := testkit.LoadFortyStepTask()
	if err != nil {
		t.Fatalf("cannot load the forty-step fixture: %v", err)
	}
	agent := startTheLiveAgent(t, model, seedTheFortyStepFixture(t, fixture))
	screen := agent.attach(t)

	checkTheFixtureAssertions(t, agent, screen, fixture, "before the model ran")
	run := agent.sendAndWaitForTheReply(t, screen, theFixtureQuestion, model.bound)
	run.report(t, model, "one answer with the fixture's record in the log")
	if strings.TrimSpace(run.reply.Text) == "" {
		t.Errorf("the model %s answered with nothing at all", model.alias)
	}
	checkTheFixtureAssertions(t, agent, screen, fixture, "after the model ran")
}

// checkTheFixtureAssertions holds the running agent to the fixture's three
// assertions, reading the record back over the socket and the results back out
// of the log the agent is writing into.
func checkTheFixtureAssertions(t *testing.T, agent runningAgent, screen *attachedScreen,
	fixture testkit.FortyStepTask, when string) {
	t.Helper()
	listed := theCommandReply(t, screen, "tasks")
	if !strings.Contains(listed, "task "+fixture.TaskID) {
		t.Fatalf("%s, the agent lists no task %s, so it never read the seeded record:\n%s",
			when, fixture.TaskID, listed)
	}

	held, _ := theRecordNumbered(t, screen, fixture.TaskID)
	if err := fixture.CheckAskAndCorrections(held); err != nil {
		t.Errorf("%s, the first assertion of the fixture fails: %v", when, err)
	}
	if err := fixture.CheckDoneList(held); err != nil {
		t.Errorf("%s, the second assertion of the fixture fails: %v", when, err)
	}
	if err := record.DoneCheck(held); err != nil {
		t.Errorf("%s, the done-check on the record the agent printed does not pass: %v", when, err)
	}
	checkEveryResultStillReadsBack(t, agent, fixture, when)
}

// checkEveryResultStillReadsBack is the third assertion: every result the task
// produced comes back in full by its id, out of the same log file the running
// agent has open.
func checkEveryResultStillReadsBack(t *testing.T, agent runningAgent, fixture testkit.FortyStepTask, when string) {
	t.Helper()
	events, err := log.Open(t.Context(), agent.home.DatabaseFile())
	if err != nil {
		t.Fatalf("%s, the agent's event log at %s cannot be opened to read the results back: %v",
			when, agent.home.DatabaseFile(), err)
	}
	defer func() {
		if err := events.Close(); err != nil {
			t.Errorf("closing the reader of the agent's event log failed: %v", err)
		}
	}()

	keeper, err := record.Load(t.Context(), events, contract.RecordTask, fixture.TaskID)
	if err != nil {
		t.Fatalf("%s, the record of task %s cannot be loaded from the agent's log: %v", when, fixture.TaskID, err)
	}
	readBack := func(id string) (string, error) { return keeper.Read(t.Context(), id) }
	if err := fixture.CheckResultsReadable(readBack); err != nil {
		t.Errorf("%s, the third assertion of the fixture fails: %v", when, err)
	}
}

// seedTheFortyStepFixture puts the fixture's whole task into the home folder's
// event log before the agent starts, so that what the live suite checks is a
// record a real agent found on disk rather than one a test holds in memory.
func seedTheFortyStepFixture(t *testing.T, fixture testkit.FortyStepTask) func(home contract.Home, work string) {
	return func(home contract.Home, _ string) {
		t.Helper()
		events, err := log.Open(t.Context(), home.DatabaseFile())
		if err != nil {
			t.Fatalf("the event log at %s cannot be made for the fixture: %v", home.DatabaseFile(), err)
		}
		playTheFixtureInto(t, events, fixture)
		if err := events.Close(); err != nil {
			t.Fatalf("closing the event log after seeding the fixture failed: %v", err)
		}
	}
}

// playTheFixtureInto plays all forty rounds into one task record: the user's
// correction at round twelve, what the model wrote into the record each round,
// and every result the round's tool produced. Last comes the closing write that
// points each done line at the result that proves it, which is what the loop
// makes the model do before a task may close.
func playTheFixtureInto(t *testing.T, store contract.Store, fixture testkit.FortyStepTask) {
	t.Helper()
	keeper, err := record.New(t.Context(), store, record.Start{
		Kind: contract.RecordTask, ID: fixture.TaskID, Origin: fixture.Origin, Ask: fixture.Ask,
		RoundsLeft: fixtureRoundsLeft, MinutesLeft: fixtureMinutesLeft,
	})
	if err != nil {
		t.Fatalf("cannot open the fixture's task record: %v", err)
	}

	for _, round := range fixture.Rounds {
		if round.Number == fixture.CorrectionRound {
			if _, err := keeper.AddCorrection(t.Context(), fixture.Correction); err != nil {
				t.Fatalf("cannot add the user's correction at round %d: %v", round.Number, err)
			}
		}
		applyTheRoundsWriting(t, keeper, round)
		if round.ToolName == "" {
			continue
		}
		addTheRoundsResults(t, keeper, fixture, round.Number)
	}

	if err := keeper.Apply(t.Context(), record.Update{DoneWhen: fixture.DoneLinesAtTheEnd()}); err != nil {
		t.Fatalf("the closing write of the fixture's done list was refused: %v", err)
	}
	if err := keeper.SetStatus(t.Context(), contract.StatusDone); err != nil {
		t.Fatalf("the fixture's record will not close, so its done list proves nothing: %v", err)
	}
}

// applyTheRoundsWriting writes the model's half of the record for one round.
func applyTheRoundsWriting(t *testing.T, keeper *record.Keeper, round testkit.FortyStepRound) {
	t.Helper()
	written, err := round.Update()
	if err != nil {
		t.Fatalf("cannot read what round %d writes into the record: %v", round.Number, err)
	}
	if written.Empty() {
		return
	}
	update := record.Update{Why: written.Why, StopWhen: written.StopWhen, Plan: written.Plan}
	for _, line := range written.DoneWhen {
		update.DoneWhen = append(update.DoneWhen, contract.DoneLine{Text: line})
	}
	if written.Decision != nil {
		update.Decision = &record.NewDecision{Text: written.Decision.Text, Reason: written.Decision.Reason}
	}
	if written.Failure != nil {
		update.Failure = &record.NewFailure{Text: written.Failure.Text, Cause: written.Failure.Cause}
	}
	if err := keeper.Apply(t.Context(), update); err != nil {
		t.Fatalf("the model's writing at round %d was refused: %v", round.Number, err)
	}
}

// addTheRoundsResults writes every result of one round into the record, and
// fails when the record labels one differently from the fixture, because the
// labels are what the third assertion reads back.
func addTheRoundsResults(t *testing.T, keeper *record.Keeper, fixture testkit.FortyStepTask, round int) {
	t.Helper()
	for _, produced := range fixture.ResultsOfRound(round) {
		id, err := keeper.AddResult(t.Context(), produced.Summary, produced.Text)
		if err != nil {
			t.Fatalf("cannot write the result of round %d into the record: %v", round, err)
		}
		if id != produced.ID {
			t.Fatalf("the record labelled round %d's result %s, and the fixture calls it %s", round, id, produced.ID)
		}
	}
}
