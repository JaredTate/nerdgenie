package loose_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/tool/loose"
)

// read parses one call's arguments the way a tool does, failing the test when
// the arguments are not an object at all.
func read(t *testing.T, written string) *loose.Fields {
	t.Helper()
	fields, err := loose.Read(json.RawMessage(written), "a path and content")
	if err != nil {
		t.Fatalf("reading %s failed: %v", written, err)
	}
	return fields
}

func TestTextIsReadWhateverShapeTheModelWroteItIn(t *testing.T) {
	shapes := []struct {
		written string
		want    string
	}{
		{`{"why":"the file is stale"}`, "the file is stale"},
		{`{"why":["the file is stale"]}`, "the file is stale"},
		{`{"why":["one line","another line"]}`, "one line\nanother line"},
		{`{"why":{"text":"the file is stale"}}`, "the file is stale"},
		{`{"why":{"value":"the file is stale"}}`, "the file is stale"},
		{`{"why":17}`, "17"},
		{`{"why":true}`, "true"},
	}
	for _, shape := range shapes {
		fields := read(t, shape.written)
		got, found := fields.Text("why")
		if !found {
			t.Errorf("%s: the field was not found at all", shape.written)
			continue
		}
		if got != shape.want {
			t.Errorf("%s: read %q, want %q", shape.written, got, shape.want)
		}
		if err := fields.Wrong(); err != nil {
			t.Errorf("%s: a shape a model writes was refused: %v", shape.written, err)
		}
	}
}

func TestAFieldWrittenUnderOneOfItsOtherNamesIsStillFound(t *testing.T) {
	fields := read(t, `{"old_string":"a line of code"}`)
	got, found := fields.Text("old", "old_string", "old_text")
	if !found || got != "a line of code" {
		t.Errorf("the field written as old_string was read as %q, found %v", got, found)
	}
}

func TestAnAbsentFieldIsToldFromAnEmptyOne(t *testing.T) {
	fields := read(t, `{"content":""}`)
	if _, found := fields.Text("content"); !found {
		t.Errorf("a content written as an empty string was read as an absent one")
	}
	fields = read(t, `{"path":"/tmp/x"}`)
	if _, found := fields.Text("content"); found {
		t.Errorf("a content that was never written was read as one that was")
	}
	fields = read(t, `{"content":null}`)
	if _, found := fields.Text("content"); found {
		t.Errorf("a content written as null was read as one the model wrote")
	}
}

func TestTheRefusalForAMissingFieldNamesTheFieldAndWhatTheModelWroteInstead(t *testing.T) {
	fields := read(t, `{"path":"/tmp/x","old_string":"a","new_string":"b"}`)
	err := fields.Missing("old", "the text to replace, quoted from the file")
	if err == nil {
		t.Fatalf("a missing field was not refused at all")
	}
	if !strings.Contains(err.Error(), "old_string") {
		t.Errorf("the refusal reads %q and does not name the key the model did write", err)
	}
	if !strings.HasSuffix(err.Error(), `"old"`) {
		t.Errorf("the refusal reads %q and does not end with the field's exact name", err)
	}
}

func TestTheRefusalForAMissingFieldWithNoNearMissStillNamesTheField(t *testing.T) {
	fields := read(t, `{"path":"/tmp/x"}`)
	err := fields.Missing("content", "the whole text the file is to hold")
	if err == nil {
		t.Fatalf("a missing field was not refused at all")
	}
	if !strings.HasSuffix(err.Error(), `"content"`) {
		t.Errorf("the refusal reads %q and does not end with the field's exact name", err)
	}
}

func TestARefusalNamesAtMostEightOfTheKeysTheModelWrote(t *testing.T) {
	written := map[string]any{}
	for at := 0; at < 40; at++ {
		written[strings.Repeat("k", at+1)] = at
	}
	asJSON, err := json.Marshal(written)
	if err != nil {
		t.Fatalf("cannot write the fixture call: %v", err)
	}
	fields := read(t, string(asJSON))
	message := fields.Missing("content", "the whole text the file is to hold").Error()
	if len(message) > 500 {
		t.Errorf("the refusal is %d characters long, and a message nobody reads is a message that does not help: %q", len(message), message)
	}
}

func TestANumberIsReadWhetherItIsQuotedOrNot(t *testing.T) {
	shapes := map[string]int{
		`{"offset":12}`:     12,
		`{"offset":"12"}`:   12,
		`{"offset":" 12 "}`: 12,
		`{"offset":12.0}`:   12,
	}
	for written, want := range shapes {
		fields := read(t, written)
		got, found := fields.Number("offset")
		if !found || got != want {
			t.Errorf("%s: read %d, found %v, want %d", written, got, found, want)
		}
		if err := fields.Wrong(); err != nil {
			t.Errorf("%s: a number a model writes was refused: %v", written, err)
		}
	}
}

func TestANumberThatIsNotOneIsRefusedByNameWithoutKillingTheWholeCall(t *testing.T) {
	fields := read(t, `{"path":"/tmp/x","offset":"the third line"}`)
	if path, _ := fields.Text("path"); path != "/tmp/x" {
		t.Errorf("one badly written field lost the fields around it, and path read as %q", path)
	}
	fields.Number("offset")
	err := fields.Wrong()
	if err == nil {
		t.Fatalf("an offset that is not a number was taken")
	}
	if !strings.Contains(err.Error(), "offset") {
		t.Errorf("the refusal reads %q and does not name the field that is wrong", err)
	}
}

func TestAFlagIsReadWhetherItIsQuotedOrNot(t *testing.T) {
	shapes := map[string]bool{
		`{"escalate":true}`:    true,
		`{"escalate":"true"}`:  true,
		`{"escalate":"TRUE"}`:  true,
		`{"escalate":"yes"}`:   true,
		`{"escalate":1}`:       true,
		`{"escalate":false}`:   false,
		`{"escalate":"false"}`: false,
		`{"escalate":"no"}`:    false,
		`{"escalate":0}`:       false,
	}
	for written, want := range shapes {
		fields := read(t, written)
		got, found := fields.Flag("escalate")
		if !found || got != want {
			t.Errorf("%s: read %v, found %v, want %v", written, got, found, want)
		}
		if err := fields.Wrong(); err != nil {
			t.Errorf("%s: a flag a model writes was refused: %v", written, err)
		}
	}
}

func TestAMarkIsReadWhetherItIsANumberOrAPageReference(t *testing.T) {
	shapes := map[string]int{
		`{"element":5}`:    5,
		`{"element":"5"}`:  5,
		`{"element":"e5"}`: 5,
		`{"element":"#5"}`: 5,
	}
	for written, want := range shapes {
		fields := read(t, written)
		got, found := fields.Mark("element")
		if !found || got != want {
			t.Errorf("%s: read %d, found %v, want %d", written, got, found, want)
		}
		if err := fields.Wrong(); err != nil {
			t.Errorf("%s: a mark a model writes was refused: %v", written, err)
		}
	}
}

func TestAnActionIsFoldedToTheOneTheModelPlainlyMeant(t *testing.T) {
	same := map[string]string{
		"CLICK":          "click",
		"Click":          "click",
		" click ":        "click",
		"set-clipboard":  "set_clipboard",
		"set clipboard":  "set_clipboard",
		"Set_Clipboard":  "set_clipboard",
		"browser_scroll": "browser_scroll",
	}
	for written, want := range same {
		if got := loose.Action(written); got != want {
			t.Errorf("the action %q folded to %q, want %q", written, got, want)
		}
	}
}

func TestArgumentsThatAreNotAnObjectAreRefusedWithWhatToWriteInstead(t *testing.T) {
	_, err := loose.Read(json.RawMessage(`"just a string"`), "a path and content")
	if err == nil {
		t.Fatalf("arguments that are not an object were taken")
	}
	if !strings.Contains(err.Error(), "a path and content") {
		t.Errorf("the refusal reads %q and does not say what to write instead", err)
	}
}

func TestNoArgumentsAtAllAreReadAsAnEmptyCall(t *testing.T) {
	fields, err := loose.Read(nil, "a path")
	if err != nil {
		t.Fatalf("a call with no arguments at all was refused: %v", err)
	}
	if _, found := fields.Text("path"); found {
		t.Errorf("a call with no arguments answered with a path")
	}
	if err := fields.Wrong(); err != nil {
		t.Errorf("a call with no arguments complained about a field: %v", err)
	}
}
