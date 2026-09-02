package contract_test

import (
	"encoding/json"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

func TestFileChangeBodyRoundTripsThroughJSONWithItsPriorContents(t *testing.T) {
	before := contract.FileChangeBody{
		Path:          "/home/someone/notes/today.md",
		Existed:       true,
		PriorContents: []byte("the old text\nwith two lines\n"),
		Mode:          0o644,
	}
	encoded, err := json.Marshal(before)
	if err != nil {
		t.Fatalf("encoding the body failed: %v", err)
	}
	var after contract.FileChangeBody
	if err := json.Unmarshal(encoded, &after); err != nil {
		t.Fatalf("decoding the body failed: %v", err)
	}
	if after.Path != before.Path || after.Existed != before.Existed || after.Mode != before.Mode {
		t.Errorf("the body came back as %+v, want %+v", after, before)
	}
	if string(after.PriorContents) != string(before.PriorContents) {
		t.Errorf("the prior contents came back as %q, want %q", after.PriorContents, before.PriorContents)
	}

	var missing contract.FileChangeBody
	if err := json.Unmarshal([]byte(`{"path":"/tmp/new.txt","existed":false}`), &missing); err != nil {
		t.Fatalf("decoding a body for a file that did not exist failed: %v", err)
	}
	if missing.Existed || missing.PriorContents != nil {
		t.Errorf("a file that did not exist should have no prior contents, got %+v", missing)
	}
}
