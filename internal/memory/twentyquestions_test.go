package memory_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// answersLookedAt is how far down a search a question's answer may be. Three is
// what the hint carries, so a fact that is not in the first three would never
// reach the model on its own.
const answersLookedAt = 3

// recordedSession is the fixture in testdata: a working week of events, the
// facts memory learned from it, and the questions a later task might ask.
type recordedSession struct {
	Task      string          `json:"task"`
	Events    []recordedEvent `json:"events"`
	Facts     []recordedFact  `json:"facts"`
	Questions []askedQuestion `json:"questions"`
}

// recordedEvent is one line of the recorded session's event log.
type recordedEvent struct {
	Kind     contract.EventKind `json:"kind"`
	Occurred time.Time          `json:"occurred"`
	Body     json.RawMessage    `json:"body"`
}

// recordedFact is one fact the session left behind.
type recordedFact struct {
	ID       string    `json:"id"`
	Text     string    `json:"text"`
	Source   string    `json:"source"`
	Recorded time.Time `json:"recorded"`
}

// askedQuestion is one question and the fact that answers it.
type askedQuestion struct {
	Ask  string `json:"ask"`
	Fact string `json:"fact"`
}

// loadRecordedSession reads the fixture out of testdata.
func loadRecordedSession(t *testing.T) recordedSession {
	t.Helper()
	held, err := os.ReadFile("testdata/twenty-questions.json")
	if err != nil {
		t.Fatalf("cannot read the recorded session: %v", err)
	}
	session := recordedSession{}
	if err := json.Unmarshal(held, &session); err != nil {
		t.Fatalf("cannot read the recorded session as JSON: %v", err)
	}
	if len(session.Facts) != 20 || len(session.Questions) != 20 {
		t.Fatalf("the recorded session holds %d facts and %d questions, and it must hold twenty of each",
			len(session.Facts), len(session.Questions))
	}
	return session
}

func TestTwentyQuestionsAgainstARecordedSession(t *testing.T) {
	ctx := context.Background()
	opened := newMemory(t, shippedCaps)
	session := loadRecordedSession(t)

	for _, event := range session.Events {
		if _, err := opened.eventLog.Append(ctx, contract.Event{
			Occurred: event.Occurred, TaskID: session.Task, Kind: event.Kind, Body: event.Body,
		}); err != nil {
			t.Fatalf("cannot write an event of the recorded session into the log: %v", err)
		}
	}
	facts := make([]contract.Fact, 0, len(session.Facts))
	for _, fact := range session.Facts {
		facts = append(facts, contract.Fact{
			ID: fact.ID, Text: fact.Text, Source: fact.Source, Recorded: fact.Recorded,
		})
	}
	if err := opened.memory.Save(ctx, facts); err != nil {
		t.Fatalf("cannot save the facts of the recorded session: %v", err)
	}
	remembering := reopen(t, opened)

	for _, question := range session.Questions {
		found, err := remembering.Search(ctx, question.Ask, answersLookedAt)
		if err != nil {
			t.Fatalf("cannot search for %q: %v", question.Ask, err)
		}
		if !answeredBy(found, question.Fact) {
			t.Errorf("the question %q was answered with %v, and %s is not among the first %d",
				question.Ask, idsOf(found), question.Fact, answersLookedAt)
		}
	}
}

// answeredBy says whether the fact that answers a question is among the results.
func answeredBy(found []contract.Fact, wanted string) bool {
	for _, fact := range found {
		if fact.ID == wanted {
			return true
		}
	}
	return false
}

// idsOf lists the ids of the results, which is what a failing question prints.
func idsOf(facts []contract.Fact) []string {
	ids := make([]string, 0, len(facts))
	for _, fact := range facts {
		ids = append(ids, fact.ID)
	}
	return ids
}
