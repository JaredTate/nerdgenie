package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestLoadingATaskThatIsNotThereIsFalse(t *testing.T) {
	store := testkit.NewFakeStore()
	if _, ok := loadTaskRecord(context.Background(), store, "404"); ok {
		t.Error("a task number nothing was written under loaded a record")
	}
}

func TestTheLatestTaskRecordHandlesAnEmptyOrMissingLog(t *testing.T) {
	if _, ok := latestTaskRecord(context.Background(), nil); ok {
		t.Error("the latest task was found with no store at all")
	}
	if _, ok := latestTaskRecord(context.Background(), testkit.NewFakeStore()); ok {
		t.Error("the latest task was found in an empty log")
	}
	if _, ok := latestTaskRecord(context.Background(), unreadableStore{Store: testkit.NewFakeStore()}); ok {
		t.Error("the latest task was found in a log that cannot be read")
	}
}

func TestCurrentOrLatestTaskIsFalseWithNoLoopAndNoLog(t *testing.T) {
	running := &agent{}
	if _, ok := running.currentOrLatestTask(context.Background()); ok {
		t.Error("a task was found with no loop and no log")
	}
}

func TestRunningJobLineIsEmptyWithNoLoop(t *testing.T) {
	running := &agent{}
	if line := running.runningJobLine(context.Background()); line != "" {
		t.Errorf("the running-job line is %q with no loop, want empty", line)
	}
}

func TestALongAskIsCutDownForTheStatus(t *testing.T) {
	long := strings.Repeat("word ", 60)
	short := shortAsk(long)
	if len([]rune(short)) > maxAskLettersInTheStatus+3 {
		t.Errorf("the cut ask is %d letters, want it near the %d cap", len([]rune(short)), maxAskLettersInTheStatus)
	}
	if !strings.HasSuffix(short, "...") {
		t.Errorf("the cut ask %q does not say the rest was cut", short)
	}
	if kept := shortAsk("a short ask"); kept != "a short ask" {
		t.Errorf("a short ask was changed to %q, and one inside the cap comes back whole", kept)
	}
}

func TestLoadingATaskKeeperThatIsNotThereIsFalse(t *testing.T) {
	if _, ok := loadTaskKeeper(context.Background(), testkit.NewFakeStore(), "404"); ok {
		t.Error("a task number nothing was written under loaded a keeper")
	}
}

// aLogWithNoTasks is a log whose reads by task all fail, the way a damaged
// file's would, so that a start time has no events to be read from.
type aLogWithNoTasks struct {
	contract.Store
}

// ByTask always fails.
func (aLogWithNoTasks) ByTask(context.Context, string) ([]contract.Event, error) {
	return nil, errors.New("the log cannot be read, so check the database file")
}

func TestTheTaskStartIsEmptyWhenTheLogCannotBeRead(t *testing.T) {
	if got := taskStartedAt(context.Background(), aLogWithNoTasks{Store: testkit.NewFakeStore()}, "7"); got != "" {
		t.Errorf("the start of a task in a log that cannot be read is %q, want empty", got)
	}
	if got := taskStartedAt(context.Background(), testkit.NewFakeStore(), "7"); got != "" {
		t.Errorf("the start of a task with no events is %q, want empty", got)
	}
}
