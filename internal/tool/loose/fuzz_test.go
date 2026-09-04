package loose_test

import (
	"encoding/json"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/tool/loose"
)

// FuzzTheLooseReader throws whatever the fuzzer writes at the reader, because
// what it reads is a model's arguments and a model writes anything at all. The
// rule it proves is that nothing here panics and every answer is bounded.
func FuzzTheLooseReader(f *testing.F) {
	f.Add(`{"path":"/tmp/x","content":"hello"}`)
	f.Add(`{"offset":"12","limit":[1,2],"element":"e5"}`)
	f.Add(`{"why":{"text":{"text":"nested"}}}`)
	f.Add(`[1,2,3]`)
	f.Add(``)

	f.Fuzz(func(t *testing.T, written string) {
		fields, err := loose.Read(json.RawMessage(written), "a path and content")
		if err != nil {
			return
		}
		fields.Text("path", "file_path")
		fields.Number("offset")
		fields.Flag("visible_only")
		fields.Mark("element")
		if message := fields.Missing("content", "the whole text the file is to hold"); len(message.Error()) > 1000 {
			t.Errorf("the refusal for a missing field is %d characters long: %q", len(message.Error()), message)
		}
		_ = fields.Wrong()
	})
}
