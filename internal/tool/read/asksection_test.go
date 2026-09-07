package read_test

import (
	"strings"
	"testing"
)

const theOrderedAsk = `# Notes app

## Goal
A notes app.

## Done when
1. Every test passes. [tests pass: npm test]

## Tasks
1. Storage. (Details: Storage)

## Details
### Storage
Notes live in local storage under one key.

### List
One line per note, newest first.
`

func TestReadsASectionOfTheAskByHeading(t *testing.T) {
	tool, _ := newTool(t, storedText{"ask": theOrderedAsk}, nil)
	output, err := run(t, tool, map[string]any{"path": "ask storage"})
	if err != nil {
		t.Fatalf("reading a section of the ask failed: %v", err)
	}
	if !strings.HasPrefix(output.Text, "### Storage") || !strings.Contains(output.Text, "under one key") {
		t.Errorf("the section came back as %q, want the Storage heading and its body", output.Text)
	}
	if strings.Contains(output.Text, "newest first") {
		t.Errorf("the section came back with the List section in it: %q", output.Text)
	}
}

func TestAnUnknownHeadingListsTheHeadings(t *testing.T) {
	tool, _ := newTool(t, storedText{"ask": theOrderedAsk}, nil)
	_, err := run(t, tool, map[string]any{"path": "ask rendering"})
	if err == nil {
		t.Fatalf("reading a section the ask does not have returned no error")
	}
	for _, heading := range []string{"Storage", "List"} {
		if !strings.Contains(err.Error(), heading) {
			t.Errorf("the refusal %q does not list the heading %q", err, heading)
		}
	}
}

func TestTheWholeAskStillComesBackByItsBareLabel(t *testing.T) {
	tool, _ := newTool(t, storedText{"ask": theOrderedAsk}, nil)
	output, err := run(t, tool, map[string]any{"path": "ask"})
	if err != nil || output.Text != theOrderedAsk {
		t.Errorf("the bare label came back with %q and error %v, want the whole ask", output.Text, err)
	}
}
