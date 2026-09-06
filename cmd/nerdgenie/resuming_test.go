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

	if picked, _ := remembered.taskToCarryOn(theTerminalScreen, "the top one"); picked != "7" {
		t.Errorf("the answer to a waiting task picked up %q, want task 7, because a question is answered by whatever the person types next", picked)
	}
}

// TestAnyMessageCarriesOnAStoppedTaskAndAllButTheBareWordSteersIt is the fix
// for the live game build's second failure. The task stopped with "Tell me how
// to carry on", the person typed "the start game button wont start game", and
// because that was not one of the four words for carrying on, a fresh task
// started with an empty record and the model asked which game. A stopped task
// ends on a question, so the next message is its answer, exactly as it is for
// a waiting task; and a message that says more than the bare word is a steer,
// written into the record as a correction so it outlives the conversation.
func TestAnyMessageCarriesOnAStoppedTaskAndAllButTheBareWordSteersIt(t *testing.T) {
	for _, said := range []string{"continue", "go on", "carry on", "keep going", "Continue.", "  KEEP GOING  "} {
		remembered := newScreenTasks()
		remembered.remember(theTerminalScreen, loop.Outcome{TaskID: "7", Status: contract.StatusStopped})
		picked, steers := remembered.taskToCarryOn(theTerminalScreen, said)
		if picked != "7" || steers {
			t.Errorf("%q after a stop picked up %q with steer %v, want task 7 and no steer, because the bare word says nothing new", said, picked, steers)
		}
	}
	for _, said := range []string{"the start game button wont start game", "write the release notes", "continue the release notes", "go", "on"} {
		remembered := newScreenTasks()
		remembered.remember(theTerminalScreen, loop.Outcome{TaskID: "7", Status: contract.StatusStopped})
		picked, steers := remembered.taskToCarryOn(theTerminalScreen, said)
		if picked != "7" || !steers {
			t.Errorf("%q after a stop picked up %q with steer %v, want task 7 steered by it: unfinished work is what the next message is about",
				said, picked, steers)
		}
	}
	remembered := newScreenTasks()
	remembered.remember(theTerminalScreen, loop.Outcome{TaskID: "7", Status: contract.StatusStopped})
	remembered.forget(theTerminalScreen)
	if picked, _ := remembered.taskToCarryOn(theTerminalScreen, "write the release notes"); picked != "" {
		t.Errorf("after /clear the next message picked up task %q, and clearing is how a person sets unfinished work aside", picked)
	}
}

func TestAFinishedOrRunningTaskIsNeverCarriedOn(t *testing.T) {
	for _, status := range []contract.RecordStatus{contract.StatusDone, contract.StatusRunning} {
		remembered := newScreenTasks()
		remembered.remember(theTerminalScreen, loop.Outcome{TaskID: "7", Status: status})
		if picked, _ := remembered.taskToCarryOn(theTerminalScreen, "continue"); picked != "" {
			t.Errorf("a task that ended %s was picked up as %q, and a finished or a still-running task is never carried on", status, picked)
		}
	}
}

// TestAFailedTaskIsCarriedOnLikeAStoppedOne holds that a task cut off by an
// error is picked up again by the next message, just as a stopped one is,
// because a failure is a stop the agent did not choose and the work behind it
// is not lost.
func TestAFailedTaskIsCarriedOnLikeAStoppedOne(t *testing.T) {
	for _, said := range []string{"continue", "carry on", "Keep going.", "try the other file"} {
		remembered := newScreenTasks()
		remembered.remember(theTerminalScreen, loop.Outcome{TaskID: "7", Status: contract.StatusFailed})
		if picked, _ := remembered.taskToCarryOn(theTerminalScreen, said); picked != "7" {
			t.Errorf("%q after a failure picked up %q, want task 7, because a failed task is resumable", said, picked)
		}
	}
}

func TestOneScreenNeverCarriesOnAnothersTask(t *testing.T) {
	remembered := newScreenTasks()
	remembered.remember(theTerminalScreen, loop.Outcome{TaskID: "7", Status: contract.StatusWaiting})

	if picked, _ := remembered.taskToCarryOn("signal:+15125550123", "the top one"); picked != "" {
		t.Errorf("a message from another screen picked up task %q, and the look-back is that screen's own newest task", picked)
	}
}

func TestOnlyTheNewestTaskOfAScreenIsCarriedOn(t *testing.T) {
	remembered := newScreenTasks()
	remembered.remember(theTerminalScreen, loop.Outcome{TaskID: "7", Status: contract.StatusWaiting})
	remembered.remember(theTerminalScreen, loop.Outcome{TaskID: "8", Status: contract.StatusDone})

	if picked, _ := remembered.taskToCarryOn(theTerminalScreen, "the top one"); picked != "" {
		t.Errorf("task %q was picked up, and only the screen's newest task is looked back at, which is task 8 and finished", picked)
	}
}

func TestATaskThatMadeNoRecordIsNeverCarriedOn(t *testing.T) {
	remembered := newScreenTasks()
	remembered.remember(theTerminalScreen, loop.Outcome{TaskID: "", Status: contract.StatusWaiting})

	if picked, _ := remembered.taskToCarryOn(theTerminalScreen, "the top one"); picked != "" {
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
	if picked, _ := remembered.taskToCarryOn("signal:0", "the top one"); picked != "" {
		t.Errorf("the screen that has been quiet longest was still remembered as %q after the cap was passed", picked)
	}
	newest := "signal:" + strconv.Itoa(maxScreensRemembered+9)
	if picked, _ := remembered.taskToCarryOn(newest, "the top one"); picked != "1" {
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
