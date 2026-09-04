package read_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/read"
)

// aLongAsk is a specification longer than the quarter of a record the model is
// shown, so that the record keeps the whole of it and shows the start with a
// line saying how to read the rest.
func aLongAsk() string {
	return strings.TrimSpace(strings.Repeat("write the whole plan out in plain words. ", 200))
}

func TestTheWholeAskComesBackByItsOwnLabel(t *testing.T) {
	keeper, err := record.New(t.Context(), testkit.NewFakeStore(), record.Start{
		Kind: contract.RecordTask, ID: "17", Ask: aLongAsk(), RoundsLeft: 40, MinutesLeft: 60,
	})
	if err != nil {
		t.Fatalf("cannot start the record: %v", err)
	}
	tool := read.New(read.Settings{Results: keeper})

	output, err := run(t, tool, map[string]any{"path": record.AskLabel})
	if err != nil {
		t.Fatalf("reading the ask by its label failed: %v", err)
	}
	if output.Text != aLongAsk() {
		t.Errorf("the ask came back as %d characters, want the whole %d the user wrote",
			len(output.Text), len(aLongAsk()))
	}
}

func TestTheSpecificationTellsTheModelThatAskIsAPathItMayRead(t *testing.T) {
	tool, _ := newTool(t, nil, nil)

	for _, field := range tool.Spec().Fields {
		if field.Name != "path" {
			continue
		}
		if !strings.Contains(field.Description, record.AskLabel) {
			t.Errorf("the path field reads %q and never says the model may read the whole ask back", field.Description)
		}
		return
	}
	t.Errorf("the tool has no path field at all")
}
