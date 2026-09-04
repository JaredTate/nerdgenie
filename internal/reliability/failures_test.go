package reliability_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/reliability"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// refusingStore is a log that takes a few events and then refuses, which is what
// a full disk looks like to the ledger.
type refusingStore struct {
	*testkit.FakeStore
	takes int
	taken int
}

// Append writes the first few events and refuses everything after them.
func (store *refusingStore) Append(ctx context.Context, event contract.Event) (int64, error) {
	store.taken++
	if store.taken > store.takes {
		return 0, errors.New("there is no room left on the disk for the log")
	}
	return store.FakeStore.Append(ctx, event)
}

// unreadableStore is a log that cannot be replayed, which is what a damaged
// database looks like to the ledger.
type unreadableStore struct{ *testkit.FakeStore }

// Replay refuses, so that the ledger cannot work out what is waiting.
func (store *unreadableStore) Replay(context.Context, func(event contract.Event) error) error {
	return errors.New("the log could not be read from the start")
}

func TestTheLedgerSaysWhenAReplyCannotBeWrittenDown(t *testing.T) {
	store := &refusingStore{FakeStore: testkit.NewFakeStore()}
	ledger := reliability.NewLedger(store, testkit.NewFakeClock(startOfTime))

	if _, err := ledger.Record(context.Background(), "17", "signal", "the post is up"); err == nil {
		t.Errorf("a reply that could not be written down was called written")
	}
}

func TestTheLedgerSaysWhenTheMarkThatAReplyArrivedCannotBeWritten(t *testing.T) {
	store := &refusingStore{FakeStore: testkit.NewFakeStore(), takes: 1}
	ledger := reliability.NewLedger(store, testkit.NewFakeClock(startOfTime))
	reply, err := ledger.Record(context.Background(), "17", "signal", "the post is up")
	if err != nil {
		t.Fatalf("writing the reply down failed: %v", err)
	}

	if err := ledger.MarkDelivered(context.Background(), reply); err == nil {
		t.Errorf("the mark that the reply arrived was lost without a word")
	}
}

func TestTheLedgerSaysWhenTheLogCannotBeRead(t *testing.T) {
	ledger := reliability.NewLedger(&unreadableStore{FakeStore: testkit.NewFakeStore()}, testkit.NewFakeClock(startOfTime))

	if _, err := ledger.Undelivered(context.Background()); err == nil {
		t.Errorf("a log that could not be read came back with nothing waiting")
	}
	if _, err := ledger.Resend(context.Background(), (&sender{}).send); err == nil {
		t.Errorf("replies were sent again out of a log that could not be read")
	}
}

func TestTheLedgerStopsAtItsOwnLimitOfWaitingReplies(t *testing.T) {
	ledger := reliability.NewLedger(testkit.NewFakeStore(), testkit.NewFakeClock(startOfTime))
	for number := range reliability.MaxUndeliveredReplies + 1 {
		if _, err := ledger.Record(context.Background(), "17", "signal", "reply "+strconv.Itoa(number)); err != nil {
			t.Fatalf("writing reply %d down failed: %v", number, err)
		}
	}

	_, err := ledger.Undelivered(context.Background())

	if err == nil {
		t.Fatalf("more than %d replies were held in memory at once", reliability.MaxUndeliveredReplies)
	}
	if !strings.Contains(err.Error(), strconv.Itoa(reliability.MaxUndeliveredReplies)) {
		t.Errorf("the failure does not say what the limit is: %v", err)
	}
}

func TestTheLedgerSaysWhenGivingUpCannotBeWrittenDown(t *testing.T) {
	store := &refusingStore{FakeStore: testkit.NewFakeStore(), takes: 1}
	clock := testkit.NewFakeClock(startOfTime)
	ledger := reliability.NewLedger(store, clock)
	if _, err := ledger.Record(context.Background(), "17", "signal", "the post is up"); err != nil {
		t.Fatalf("writing the reply down failed: %v", err)
	}
	clock.Advance(reliability.DeliveryLifetime + time.Minute)

	if _, err := ledger.Resend(context.Background(), (&sender{}).send); err == nil {
		t.Errorf("giving up on a reply was not written down and nothing said so")
	}
}

func TestTheLedgerSaysWhenAnAttemptCannotBeWrittenDown(t *testing.T) {
	store := &refusingStore{FakeStore: testkit.NewFakeStore(), takes: 1}
	ledger := reliability.NewLedger(store, testkit.NewFakeClock(startOfTime))
	if _, err := ledger.Record(context.Background(), "17", "signal", "the post is up"); err != nil {
		t.Fatalf("writing the reply down failed: %v", err)
	}

	if _, err := ledger.Resend(context.Background(), (&sender{}).send); err == nil {
		t.Errorf("a reply was sent again without the attempt being written down first")
	}
}

func TestTheBreakerSaysWhenItsStateFileCannotBeRemoved(t *testing.T) {
	home := testkit.NewTempHome(t)
	inTheWay := filepath.Join(home.RunFolder(), reliability.BreakerFileName)
	writeFile(t, filepath.Join(inTheWay, "something"), "a folder where the file goes", contract.DataFileMode)

	if err := reliability.NewBreaker(home, testkit.NewFakeClock(startOfTime)).Clear(); err == nil {
		t.Errorf("a state file that could not be removed was called cleared")
	}
}

func TestAStateFileTooLongToBeOneOfOursIsRefused(t *testing.T) {
	home := testkit.NewTempHome(t)
	tooMuch := strings.Repeat("x", 2<<20)
	writeFile(t, filepath.Join(home.RunFolder(), reliability.BreakerFileName), tooMuch, contract.DataFileMode)

	_, err := reliability.NewBreaker(home, testkit.NewFakeClock(startOfTime)).Tripped()

	if err == nil {
		t.Fatalf("a file of %d bytes was read as the breaker's own state", len(tooMuch))
	}
	if !strings.Contains(err.Error(), "longer than") {
		t.Errorf("the failure does not say that the file is too long: %v", err)
	}
}

func TestADatabaseCannotBeMovedOnTopOfOneThatIsAlreadyThere(t *testing.T) {
	folder := t.TempDir()
	path := filepath.Join(folder, "nerdgenie.db")
	writeFile(t, path, "not a database at all", contract.SecretFileMode)
	movedTo, err := reliability.MoveDatabaseAside(path, startOfTime)
	if err != nil {
		t.Fatalf("moving the database aside failed: %v", err)
	}
	writeFile(t, path, "another database that went wrong", contract.SecretFileMode)

	_, err = reliability.MoveDatabaseAside(path, startOfTime)

	if err == nil {
		t.Fatalf("the second damaged database was written over the first one at %s", movedTo)
	}
	if !strings.Contains(err.Error(), movedTo) {
		t.Errorf("the failure does not name what is already there: %v", err)
	}
}

func TestTheGuardSaysWhenItCannotWriteItsOwnFiles(t *testing.T) {
	home := contract.NewHome(filepath.Join(t.TempDir(), "cannot-be-written-in"))
	if err := os.Mkdir(home.Root, 0o500); err != nil {
		t.Fatalf("making the read-only home failed: %v", err)
	}
	guard, _ := aGuard(t, home, testkit.NewFakeStore())

	if _, err := guard.Start(context.Background()); err == nil {
		t.Errorf("the guard started in a home it cannot write in")
	}
}

func TestTheGuardWritesTheNightlyBackup(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	guard, _ := aGuard(t, home, testkit.NewFakeStore())

	archive, err := guard.Backup(context.Background())
	if err != nil {
		t.Fatalf("the nightly backup failed: %v", err)
	}

	if _, err := os.Stat(archive); err != nil {
		t.Errorf("the nightly backup wrote no archive: %v", err)
	}
}

func TestALeaseKnowsWhichSessionItIsHeldFor(t *testing.T) {
	leases := reliability.NewLeases(testkit.NewFakeClock(startOfTime))
	lease, err := leases.Acquire(context.Background(), "signal")
	if err != nil {
		t.Fatalf("taking the lease failed: %v", err)
	}
	defer lease.Release()

	if lease.Session() != "signal" {
		t.Errorf("the lease is held for %q, want signal", lease.Session())
	}
}

func TestADeadlineWithATimeInThePastHasAlreadyRunOut(t *testing.T) {
	clock := testkit.NewFakeClock(startOfTime)
	deadline := reliability.NewDeadline(clock, "the turn", -time.Hour)

	if !deadline.Expired() || deadline.Remaining() != 0 {
		t.Errorf("a deadline made with a limit in the past has %s left", deadline.Remaining())
	}

	clock.Advance(time.Hour)
	if deadline.Remaining() != 0 {
		t.Errorf("a deadline an hour past its time has %s left", deadline.Remaining())
	}
}

func TestTheGuardWritesTheNightlyBackupWhereTheConfigurationSaid(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	folder := filepath.Join(t.TempDir(), "somewhere", "else")
	told := &sender{}
	guard, err := reliability.New(reliability.Settings{
		Home:         home,
		Clock:        testkit.NewFakeClock(startOfTime),
		Store:        testkit.NewFakeStore(),
		Caps:         contract.DefaultConfig().Caps,
		BackupFolder: folder,
		Send:         told.send,
	})
	if err != nil {
		t.Fatalf("building the guard failed: %v", err)
	}

	archive, err := guard.Backup(context.Background())
	if err != nil {
		t.Fatalf("the nightly backup failed: %v", err)
	}

	if filepath.Dir(archive) != folder {
		t.Errorf("the nightly backup went to %s, want the folder the configuration named, %s", archive, folder)
	}
}

func TestTheGuardSaysWhenTheBreakerCannotBeRead(t *testing.T) {
	home := testkit.NewTempHome(t)
	writeFile(t, filepath.Join(home.RunFolder(), reliability.BreakerFileName), "half a file", contract.DataFileMode)
	guard, _ := aGuard(t, home, testkit.NewFakeStore())

	if _, err := guard.Start(context.Background()); err == nil {
		t.Errorf("the guard started without saying that it could not read the breaker")
	}
}

func TestAStateFileSaysWhenItCannotBeWrittenIntoTheRunFolder(t *testing.T) {
	home := testkit.NewTempHome(t)
	if err := os.Chmod(home.RunFolder(), 0o500); err != nil {
		t.Fatalf("making the run folder read-only failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(home.RunFolder(), contract.HomeFolderMode) })

	_, err := reliability.NewBreaker(home, testkit.NewFakeClock(startOfTime)).RecordUncleanStart()

	if err == nil {
		t.Errorf("a state file was written into a folder nobody can write in")
	}
}

func TestTheWatchdogSaysWhenSystemdAnnouncedSomethingItCannotRead(t *testing.T) {
	t.Setenv("WATCHDOG_USEC", "not a number of microseconds")
	t.Setenv("WATCHDOG_PID", strconv.Itoa(os.Getpid()))

	if _, err := reliability.NewWatchdog(testkit.NewFakeClock(startOfTime)); err == nil {
		t.Errorf("an interval that is not a number was read as one")
	}
}

func TestTheWatchdogSaysWhenTheServiceManagerCannotBeReached(t *testing.T) {
	folder, err := os.MkdirTemp("", "nerdgenie-notify-*")
	if err != nil {
		t.Fatalf("making the folder failed: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(folder) })
	t.Setenv("NOTIFY_SOCKET", filepath.Join(folder, "nothing-is-listening.sock"))
	t.Setenv("WATCHDOG_USEC", strconv.FormatInt(watchdogInterval.Microseconds(), 10))
	t.Setenv("WATCHDOG_PID", strconv.Itoa(os.Getpid()))
	clock := testkit.NewFakeClock(startOfTime)
	watchdog, err := reliability.NewWatchdog(clock)
	if err != nil {
		t.Fatalf("building the watchdog failed: %v", err)
	}

	if err := watchdog.Ready(); err == nil {
		t.Errorf("saying the program is ready to a socket nobody is listening on was called a success")
	}

	stopped := make(chan error, 1)
	go func() { stopped <- watchdog.Feed(context.Background()) }()
	for range 100 {
		clock.Advance(watchdogInterval / 2)
		select {
		case err := <-stopped:
			if err == nil {
				t.Errorf("the feed stopped without saying that the service manager could not be reached")
			}
			return
		default:
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("the feed never noticed that the service manager could not be reached")
}
