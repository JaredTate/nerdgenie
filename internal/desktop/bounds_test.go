package desktop

import (
	"testing"
	"time"
)

// Every number this package is bounded by is written out here as a literal,
// beside one sentence saying why it is that number. A test that measures against
// the constant that decides it moves with the constant, so it proves nothing: a
// deadline raised from thirty seconds to thirty hours left this package green,
// and a task waiting thirty hours on one click is a task that never finishes.

func TestEveryMethodDeadlineIsTheNumberItIsMeantToBe(t *testing.T) {
	// Launching waits for a window to appear and a screenshot reads the whole
	// window and encodes a picture, so those two get thirty seconds. Typing is
	// paced like a person and a long post takes a while, so it gets a minute.
	// A click, a key press and a drag are one movement each, and the clipboard
	// and the health check are a question and an answer.
	wanted := map[string]time.Duration{
		"launch":       30 * time.Second,
		"screenshot":   30 * time.Second,
		"click":        20 * time.Second,
		"type":         60 * time.Second,
		"press":        20 * time.Second,
		"drag":         20 * time.Second,
		"clipboardGet": 10 * time.Second,
		"clipboardSet": 10 * time.Second,
		"health":       10 * time.Second,
	}
	if len(methodDeadlines) != len(wanted) {
		t.Errorf("the table names %d methods, want the %d of the protocol, each with a deadline written down here",
			len(methodDeadlines), len(wanted))
	}
	for method, want := range wanted {
		if got := deadlineFor(method); got != want {
			t.Errorf("the deadline for %s is %s, want %s", method, got, want)
		}
	}
	// Twenty seconds for a method the table does not name, which is what one
	// movement on the screen takes.
	if deadlineFor("something nobody named") != 20*time.Second {
		t.Errorf("a method the table does not name waits %s, want 20s", deadlineFor("something nobody named"))
	}
}

func TestNoDeadlineOnThisPackageIsLongerThanAMinute(t *testing.T) {
	// A person watching a task can wait a minute for one step on the screen and
	// no longer, and a worker that has not answered in a minute is a worker to
	// start again rather than one to go on waiting for.
	const longest = time.Minute
	for method, deadline := range methodDeadlines {
		if deadline > longest {
			t.Errorf("the deadline for %s is %s, and no step on the desktop may wait longer than %s", method, deadline, longest)
		}
		if deadline <= 0 {
			t.Errorf("the deadline for %s is %s, and every wait in Coeus has one", method, deadline)
		}
	}
	if defaultMethodDeadline > longest || defaultMethodDeadline <= 0 {
		t.Errorf("a method the table does not name waits %s, want something above nothing and no longer than %s", defaultMethodDeadline, longest)
	}
}

func TestEveryByteCapAndEveryWaitIsTheNumberItIsMeantToBe(t *testing.T) {
	numbers := []struct {
		name string
		got  int
		want int
		why  string
	}{
		{
			name: "maximumResponseBytes",
			got:  maximumResponseBytes,
			want: 8 << 20,
			why:  "the worker refuses a picture over four megabytes of base64 text, so eight is room for that and the marks beside it",
		},
		{
			name: "responseReaderBytes",
			got:  responseReaderBytes,
			want: 64 * 1024,
			why:  "one answer line holds a whole window's controls, and sixty-four kilobytes reads almost all of them in one go",
		},
		{
			name: "maximumLogLineLength",
			got:  maximumLogLineLength,
			want: 4096,
			why:  "one line of the worker's own log, which is a sentence and a stack line, never a picture",
		},
	}
	for _, number := range numbers {
		if number.got != number.want {
			t.Errorf("%s is %d, want %d, because %s", number.name, number.got, number.want, number.why)
		}
	}
	// Two seconds for a worker to go quietly after it is asked to, which is long
	// enough for Node to close its streams and short enough that nobody waits.
	if killGracePeriod != 2*time.Second {
		t.Errorf("a worker is given %s to go quietly before it is made to go, want 2s", killGracePeriod)
	}
}
