package workorder_test

import (
	"testing"

	"github.com/JaredTate/nerdgenie/internal/workorder"
)

// Run 23's visual QA task spent forty rounds proving by hand that the board
// was square, centred and console-clean at three widths. A fifth check lets
// the harness prove that itself: "[looks: url at 1440, 768, 390]" opens the
// page at each width and reads whether it overflows and whether the console
// holds an error.
func TestALooksCheckNamesTheAddressAndTheWidths(t *testing.T) {
	for _, shape := range []struct {
		line   string
		url    string
		widths []int
	}{
		{"The board fits every window. [looks: http://127.0.0.1:8097 at 1440, 768, 390]", "http://127.0.0.1:8097", []int{1440, 768, 390}},
		{"The page fits. [looks: http://127.0.0.1:8097 at 390]", "http://127.0.0.1:8097", []int{390}},
		{"The page fits. [looks: http://127.0.0.1:8097]", "http://127.0.0.1:8097", []int{1440}},
		{"The page fits. [Looks:  http://127.0.0.1:8097/play  at  768 , 1440 ]", "http://127.0.0.1:8097/play", []int{768, 1440}},
	} {
		check, found := workorder.ReadCheck(shape.line)
		if !found {
			t.Errorf("ReadCheck(%q) found no check", shape.line)
			continue
		}
		if check.Kind != "looks" || check.URL != shape.url || !sameWidths(check.Widths, shape.widths) {
			t.Errorf("ReadCheck(%q) = %+v, want kind looks, url %q, widths %v", shape.line, check, shape.url, shape.widths)
		}
	}
}

// TestALooksCheckIsBounded: at most four widths, each between 320 and 3840,
// on an address that is one; anything else is words, not a check.
func TestALooksCheckIsBounded(t *testing.T) {
	for _, line := range []string{
		"The page fits. [looks: http://127.0.0.1:8097 at 100]",
		"The page fits. [looks: http://127.0.0.1:8097 at 4000]",
		"The page fits. [looks: http://127.0.0.1:8097 at wide]",
		"The page fits. [looks: http://127.0.0.1:8097 at 1440, 1280, 1024, 768, 390]",
		"The page fits. [looks: the game at 1440]",
		"The page fits. [looks: at 1440]",
	} {
		if check, found := workorder.ReadCheck(line); found {
			t.Errorf("ReadCheck(%q) read a check %+v, want none", line, check)
		}
	}
}

func sameWidths(got []int, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for at := range got {
		if got[at] != want[at] {
			return false
		}
	}
	return true
}
