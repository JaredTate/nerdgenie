package record

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// storedResultBodies reads back every result the keeper wrote into the log,
// as the raw JSON and as the struct it decodes to.
func storedResultBodies(t *testing.T, store *testkit.FakeStore, keeper *Keeper) ([]string, []StoredResult) {
	t.Helper()
	events, err := store.ByTask(t.Context(), keeper.LogKey())
	if err != nil {
		t.Fatalf("cannot read the log back: %v", err)
	}
	raw, decoded := []string{}, []StoredResult{}
	for _, event := range events {
		if event.Kind != contract.EventToolResult {
			continue
		}
		stored := StoredResult{}
		if err := json.Unmarshal(event.Body, &stored); err != nil {
			t.Fatalf("cannot decode the stored result %s: %v", event.Body, err)
		}
		raw, decoded = append(raw, string(event.Body)), append(decoded, stored)
	}
	return raw, decoded
}

// TestAResultKeepsTheIDOfTheCallThatMadeIt holds that a result added for a
// tool call carries the call's id in the log, so whoever reads the result
// later can find the call that made it without guessing from its place.
func TestAResultKeepsTheIDOfTheCallThatMadeIt(t *testing.T) {
	keeper, store := newKeeper(t, taskStart())

	label, err := keeper.AddResultOfCall(t.Context(), "call_7", "read the notes", "the notes")
	if err != nil {
		t.Fatalf("cannot add the result of the call: %v", err)
	}

	raw, decoded := storedResultBodies(t, store, keeper)
	if len(decoded) != 1 || decoded[0].ID != label || decoded[0].CallID != "call_7" {
		t.Fatalf("the log holds %+v, want one result %s made by call_7", decoded, label)
	}
	if !strings.Contains(raw[0], `"callId":"call_7"`) {
		t.Errorf("the stored body is %s, want the call id under callId", raw[0])
	}
	if text, err := keeper.Read(t.Context(), label); err != nil || text != "the notes" {
		t.Errorf("the result reads back as %q, %v", text, err)
	}
}

// TestAResultNoCallMadeCarriesNoCallID holds that a result the harness writes
// on its own, such as the reply that proves a done list, carries no call id at
// all, so an old log and a new one read the same way.
func TestAResultNoCallMadeCarriesNoCallID(t *testing.T) {
	keeper, store := newKeeper(t, taskStart())

	if _, err := keeper.AddResult(t.Context(), "the reply to the user: done", "done"); err != nil {
		t.Fatalf("cannot add the result: %v", err)
	}

	raw, decoded := storedResultBodies(t, store, keeper)
	if len(decoded) != 1 || decoded[0].CallID != "" {
		t.Fatalf("the log holds %+v, want one result with no call id", decoded)
	}
	if strings.Contains(raw[0], "callId") {
		t.Errorf("the stored body is %s, and a result no call made must not carry a callId field", raw[0])
	}
}
