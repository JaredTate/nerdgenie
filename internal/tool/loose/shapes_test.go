package loose_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/tool/loose"
)

func TestAFieldWrittenAsSomethingWithNoTextInItIsRefusedByName(t *testing.T) {
	shapes := []string{
		`{"content":[{"kind":"paragraph"}]}`,
		`{"content":{"kind":"paragraph"}}`,
		`{"content":[]}`,
	}
	for _, written := range shapes {
		fields := read(t, written)
		if text, found := fields.Text("content"); found {
			t.Errorf("%s: read %q as the content, and there is no text in it", written, text)
		}
		err := fields.Wrong()
		if err == nil {
			t.Errorf("%s: a field with no text in it was passed over in silence", written)
			continue
		}
		if !strings.Contains(err.Error(), "content") {
			t.Errorf("%s: the refusal reads %q and does not name the field", written, err)
		}
	}
}

func TestAListNestedDeeperThanTheReaderLooksIsRefusedRatherThanFollowed(t *testing.T) {
	fields := read(t, `{"content":[[[["far too deep"]]]]}`)
	if _, found := fields.Text("content"); found {
		t.Errorf("a list nested four deep was followed, and the reader stops at %d", loose.MaxNesting)
	}
	if fields.Wrong() == nil {
		t.Errorf("a list nested too deep was passed over in silence")
	}
}

func TestAFieldWrittenUnderAKeyOfADifferentCaseIsStillFound(t *testing.T) {
	spellings := []string{"File-Path", "filePath", "FILE_PATH", "file path"}
	for _, spelling := range spellings {
		fields := read(t, `{"`+spelling+`":"/tmp/x"}`)
		if path, found := fields.Text("file_path"); !found || path != "/tmp/x" {
			t.Errorf("the field written as %s read as %q, found %v", spelling, path, found)
		}
	}
}

func TestAMarkAndAFlagThatAreNeitherAreRefusedByName(t *testing.T) {
	fields := read(t, `{"element":{"role":"button"},"visible_only":"perhaps"}`)
	if _, found := fields.Mark("element"); found {
		t.Errorf("an element written as an object was read as the number of a control")
	}
	if _, found := fields.Flag("visible_only"); found {
		t.Errorf("a flag written as \"perhaps\" was read as true or false")
	}
	err := fields.Wrong()
	if err == nil {
		t.Fatalf("neither badly written field was refused")
	}
	if !strings.Contains(err.Error(), "element") || !strings.Contains(err.Error(), "visible_only") {
		t.Errorf("the refusal reads %q and does not name both fields", err)
	}
}

func TestAValueLongerThanTheRefusalShowsIsCutInIt(t *testing.T) {
	fields := read(t, `{"offset":"`+strings.Repeat("x", 300)+`"}`)
	fields.Number("offset")
	err := fields.Wrong()
	if err == nil {
		t.Fatalf("an offset that is 300 letters long was taken as a number")
	}
	if strings.Contains(err.Error(), strings.Repeat("x", loose.MaxShownRunes+1)) {
		t.Errorf("the refusal shows the whole of a 300-letter value: %q", err)
	}
	if !strings.Contains(err.Error(), "...") {
		t.Errorf("the refusal reads %q and does not say the value was cut", err)
	}
}
