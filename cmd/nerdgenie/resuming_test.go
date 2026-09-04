package main

import (
	"strconv"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
)

// theTerminalScreen is the name one screen goes by in these tests: a channel and
// a sender, which is how the wiring tells one screen from another.
const theTerminalScreen = "terminal:terminal"

func TestAnyMessageCarriesOnTheScreensWaitingTask(t *testing.T) {
	remembered := newScreenTasks()
	remembered.remember(theTerminalScreen, loop.Outcome{TaskID: "7", Status: contract.StatusWaiting})

	if picked := remembered.taskToCarryOn(theTerminalScreen, "the top one"); picked != "7" {
		t.Errorf("the answer to a waiting task picked up %q, want task 7, because a question is answered by whatever the person types next", picked)
	}
}

func TestOnlyTheWordsForCarryingOnPickUpAStoppedTask(t *testing.T) {
	for _, said := range []string{"continue", "go on", "carry on", "keep going", "Continue.", "  KEEP GOING  "} {
		remembered := newScreenTasks()
		remembered.remember(theTerminalScreen, loop.Outcome{TaskID: "7", Status: contract.StatusStopped})
		if picked := remembered.taskToCarryOn(theTerminalScreen, said); picked != "7" {
			t.Errorf("%q after a stop picked up %q, want task 7", said, picked)
		}
	}
	for _, said := range []string{"write the release notes", "continue the release notes", "go", "", "on"} {
		remembered := newScreenTasks()
		remembered.remember(theTerminalScreen, loop.Outcome{TaskID: "7", Status: contract.StatusStopped})
		if picked := remembered.taskToCarryOn(theTerminalScreen, said); picked != "" {
			t.Errorf("%q after a stop picked up task %q, and a message that is not one of the words for carrying on is a new ask",
				said, picked)
		}
	}
}

func TestAFinishedOrFailedTaskIsNeverCarriedOn(t *testing.T) {
	for _, status := range []contract.RecordStatus{contract.StatusDone, contract.StatusFailed, contract.StatusRunning} {
		remembered := newScreenTasks()
		remembered.remember(theTerminalScreen, loop.Outcome{TaskID: "7", Status: status})
		if picked := remembered.taskToCarryOn(theTerminalScreen, "continue"); picked != "" {
			t.Errorf("a task that ended %s was picked up as %q, and only a waiting or a stopped task is carried on", status, picked)
		}
	}
}

func TestOneScreenNeverCarriesOnAnothersTask(t *testing.T) {
	remembered := newScreenTasks()
	remembered.remember(theTerminalScreen, loop.Outcome{TaskID: "7", Status: contract.StatusWaiting})

	if picked := remembered.taskToCarryOn("signal:+15125550123", "the top one"); picked != "" {
		t.Errorf("a message from another screen picked up task %q, and the look-back is that screen's own newest task", picked)
	}
}

func TestOnlyTheNewestTaskOfAScreenIsCarriedOn(t *testing.T) {
	remembered := newScreenTasks()
	remembered.remember(theTerminalScreen, loop.Outcome{TaskID: "7", Status: contract.StatusWaiting})
	remembered.remember(theTerminalScreen, loop.Outcome{TaskID: "8", Status: contract.StatusDone})

	if picked := remembered.taskToCarryOn(theTerminalScreen, "the top one"); picked != "" {
		t.Errorf("task %q was picked up, and only the screen's newest task is looked back at, which is task 8 and finished", picked)
	}
}

func TestATaskThatMadeNoRecordIsNeverCarriedOn(t *testing.T) {
	remembered := newScreenTasks()
	remembered.remember(theTerminalScreen, loop.Outcome{TaskID: "", Status: contract.StatusWaiting})

	if picked := remembered.taskToCarryOn(theTerminalScreen, "the top one"); picked != "" {
		t.Errorf("a task with no record was picked up as %q, and there is no record to pick up", picked)
	}
}

func TestTheScreensRememberedAreBounded(t *testing.T) {
	remembered := newScreenTasks()
	for screen := range maxScreensRemembered + 10 {
		remembered.remember("signal:"+strconv.Itoa(screen), loop.Outcome{TaskID: "1", Status: contract.StatusWaiting})
	}

	if held := remembered.howManyScreens(); held > maxScreensRemembered {
		t.Errorf("%d screens are remembered, and the cap is %d, so a channel with many senders would grow without end",
			held, maxScreensRemembered)
	}
	if picked := remembered.taskToCarryOn("signal:0", "the top one"); picked != "" {
		t.Errorf("the screen that has been quiet longest was still remembered as %q after the cap was passed", picked)
	}
	newest := "signal:" + strconv.Itoa(maxScreensRemembered+9)
	if picked := remembered.taskToCarryOn(newest, "the top one"); picked != "1" {
		t.Errorf("the newest screen picked up %q, want task 1, because the cap forgets the oldest and not the newest", picked)
	}
}

func TestASecondMessageFromAScreenReplacesWhatIsRememberedRatherThanAddingToIt(t *testing.T) {
	remembered := newScreenTasks()
	for range maxScreensRemembered + 10 {
		remembered.remember(theTerminalScreen, loop.Outcome{TaskID: "7", Status: contract.StatusWaiting})
	}

	if held := remembered.howManyScreens(); held != 1 {
		t.Errorf("one screen sending many messages is remembered as %d screens, want one", held)
	}
}
