package main

import (
	"context"
	"strings"
	"testing"
)

// TestAShowOfAResultTakesTheCallItNamesOverTheNearestOne holds that a result
// stored with the id of the call that made it is shown with that call, even
// when another call sits nearer to it in the log, which is what happens when a
// call is written and refused before the result of the one before it lands.
func TestAShowOfAResultTakesTheCallItNamesOverTheNearestOne(t *testing.T) {
	answering, keeper := aShowingOverFakes(t)
	ctx := context.Background()
	logAToolCall(t, answering.store, "6", "read", `{"path":"notes.md"}`)
	logAToolCall(t, answering.store, "6", "write", `{"path":"out.md"}`)
	label, err := keeper.AddResultOfCall(ctx, "call_read", "read the notes", "the notes")
	if err != nil {
		t.Fatalf("cannot add the result of the read call: %v", err)
	}

	got, err := answering.answer(ctx, map[string]string{"id": label, "task": "6"})
	if err != nil {
		t.Fatalf("showing %s of task 6 failed: %v", label, err)
	}

	if !strings.HasPrefix(got.Text, "call: read\n") {
		t.Errorf("the shown text begins %q, want the read call the result names", firstLineOf(got.Text))
	}
}
