package desktop

import (
	"context"
	"fmt"
	"strings"
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
			t.Errorf("the deadline for %s is %s, and every wait in Nerd Genie has one", method, deadline)
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
		{
			name: "MaxMarksKept",
			got:  MaxMarksKept,
			want: 200,
			why:  "the model reads every mark as a line of its context, and two hundred lines is a long window already",
		},
		{
			name: "MaxGrantedApplications",
			got:  MaxGrantedApplications,
			want: 20,
			why:  "a task works in one application or two, and twenty is far past what a person would grant in one session",
		},
		{
			name: "MaxApplicationNameRunes",
			got:  MaxApplicationNameRunes,
			want: 120,
			why:  "an application name is a program's name or a window's title, and the user reads it in a preview",
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

func TestOnlyAsManyMarksAsTheModelCanReadComeBack(t *testing.T) {
	desk := newDesk(t)
	desk.launched(t)
	crowded := []any{}
	for number := 1; number <= 50_000; number++ {
		crowded = append(crowded, map[string]any{"number": number, "role": "button", "name": "OK"})
	}
	desk.latestWorker(t).answer("screenshot", map[string]any{"pngBase64": "iVBORw0KGgoFAKE", "marks": crowded})

	picture, err := desk.desktop.Screenshot(context.Background())

	if err != nil {
		t.Fatalf("taking a screenshot of a crowded window failed: %v", err)
	}
	if len(picture.Marks) != 200 {
		t.Errorf("a window with fifty thousand controls came back with %d marks, want 200: every one of them is printed into the model's"+
			" context, and a window that crowded would fill a small model's whole window", len(picture.Marks))
	}
}

func TestOnlyAsManyApplicationsAsAPersonWouldGrantAreKept(t *testing.T) {
	desk := newDesk(t)
	ctx := context.Background()
	for number := 1; number <= 20; number++ {
		name := fmt.Sprintf("application-%d", number)
		if err := desk.desktop.Launch(ctx, name, "a window opens"); err != nil {
			t.Fatalf("granting %s failed: %v", name, err)
		}
	}

	err := desk.desktop.Launch(ctx, "one-application-too-many", "a window opens")

	if err == nil || !strings.Contains(err.Error(), "20") {
		t.Fatalf("the twenty-first application gave %v, want an error naming how many one session may grant", err)
	}
}

func TestAnApplicationNameLongerThanThePreviewCanShowIsRefused(t *testing.T) {
	desk := newDesk(t)

	err := desk.desktop.Launch(context.Background(), strings.Repeat("z", 5000), "a window opens")

	if err == nil || !strings.Contains(err.Error(), "120") {
		t.Fatalf("a name of five thousand characters gave %v, want an error saying how long a name may be", err)
	}
	if shown := len(desk.channel.Previews()); shown != 0 {
		t.Errorf("the user was shown %d previews of a name that long, want none: the preview is a sentence a person reads", shown)
	}
}

func TestOnlyAsManyWindowTitlesAsTheModelCanReadComeBack(t *testing.T) {
	desk := newDesk(t)
	desk.launched(t)
	crowded := []any{}
	for number := 1; number <= 5_000; number++ {
		crowded = append(crowded, fmt.Sprintf("Window %d", number))
	}
	desk.latestWorker(t).answer("screenshot", map[string]any{"pngBase64": "iVBORw0KGgoFAKE", "marks": []any{}, "windows": crowded})

	picture, err := desk.desktop.Screenshot(context.Background())

	if err != nil {
		t.Fatalf("taking a screenshot of a crowded screen failed: %v", err)
	}
	if len(picture.Windows) != 50 {
		t.Errorf("a screen with five thousand windows came back naming %d of them, want 50: every title is a line of the model's"+
			" context, and a person has no more windows open than that", len(picture.Windows))
	}
}
