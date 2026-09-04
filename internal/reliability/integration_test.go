//go:build integration

package reliability_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/log"
	"github.com/JaredTate/nerdgenie/internal/reliability"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// helperVariable names the database the helper process writes a reply into. The
// helper is this same test binary, run again with the variable set, so that the
// test can kill a real process in the middle of a real turn.
const helperVariable = "COEUS_TEST_LEDGER_HELPER"

// helperReady is what the helper prints once the reply is in the log and it is
// safe to kill it.
const helperReady = "the reply is written"

// aRealLog opens the event log in a real SQLite file and closes it when the test
// is over.
func aRealLog(t *testing.T, path string) *log.Log {
	t.Helper()
	opened, err := log.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("opening the event log at %s failed: %v", path, err)
	}
	t.Cleanup(func() { _ = opened.Close() })
	return opened
}

// TestTheHelperWritesAReplyAndWaitsToBeKilled is not a test of its own. It is
// the helper process: run with the variable set, it writes a reply into the log
// the way a turn does and then waits to be killed, which is what leaves a reply
// that was written down and never delivered.
func TestTheHelperWritesAReplyAndWaitsToBeKilled(t *testing.T) {
	path := os.Getenv(helperVariable)
	if path == "" {
		t.Skip("this is the helper process for the kill test, and it only runs when the test starts it")
	}
	home := contract.NewHome(filepath.Dir(path))
	databaseFile, err := reliability.PrepareDatabase(context.Background(), reliability.RecoverySettings{
		Home:  home,
		Clock: testkit.NewFakeClock(startOfTime),
	})
	if err != nil {
		t.Fatalf("the helper could not prepare the database: %v", err)
	}
	opened, err := log.Open(context.Background(), databaseFile)
	if err != nil {
		t.Fatalf("the helper could not open the log: %v", err)
	}
	guard, err := reliability.New(reliability.Settings{
		Home:  home,
		Clock: testkit.NewFakeClock(startOfTime),
		Store: opened,
		Caps:  contract.DefaultConfig().Caps,
		Send:  func(context.Context, string, string) error { return nil },
	})
	if err != nil {
		t.Fatalf("the helper could not build the guard: %v", err)
	}
	if _, err := guard.Start(context.Background()); err != nil {
		t.Fatalf("the helper could not start: %v", err)
	}
	if _, err := guard.Ledger().Record(context.Background(), "17", "signal", "the post is up"); err != nil {
		t.Fatalf("the helper could not write the reply down: %v", err)
	}

	fmt.Println(helperReady)
	// The reply is written and nothing has marked it delivered. Wait here to be
	// killed, under a limit so that a helper nobody kills does not live forever.
	time.Sleep(2 * time.Minute)
}

func TestAReplyWrittenByAProgramThatWasKilledIsSentAgainWithTheMarker(t *testing.T) {
	home := testkit.NewTempHome(t)
	helper := exec.Command(os.Args[0], "-test.run=TestTheHelperWritesAReplyAndWaitsToBeKilled", "-test.timeout=3m")
	helper.Env = append(os.Environ(), helperVariable+"="+home.DatabaseFile())
	printed, err := helper.StdoutPipe()
	if err != nil {
		t.Fatalf("listening to the helper failed: %v", err)
	}
	if err := helper.Start(); err != nil {
		t.Fatalf("starting the helper failed: %v", err)
	}
	waitForTheHelper(t, printed)

	// Kill the exact process this test started, by the process id it was given,
	// and never by a pattern: a pattern matches this test as well.
	if err := helper.Process.Kill(); err != nil {
		t.Fatalf("killing the helper failed: %v", err)
	}
	_ = helper.Wait()

	told := &sender{}
	guard, err := reliability.New(reliability.Settings{
		Home:  home,
		Clock: testkit.NewFakeClock(startOfTime),
		Store: aRealLog(t, aNewLife(t, home)),
		Caps:  contract.DefaultConfig().Caps,
		Send:  told.send,
	})
	if err != nil {
		t.Fatalf("building the guard after the kill failed: %v", err)
	}

	found, err := guard.Start(context.Background())
	if err != nil {
		t.Fatalf("starting after the kill failed: %v", err)
	}

	if !found.UncleanExit {
		t.Errorf("the start after a killed program was called clean")
	}
	if found.RepliesSentAgain != 1 {
		t.Fatalf("%d replies were sent again, want the one the killed program never delivered", found.RepliesSentAgain)
	}
	if len(told.sent) != 1 || !strings.HasPrefix(told.sent[0].text, reliability.DuplicateMarker) {
		t.Fatalf("the reply came back as %q, want it marked as a possible duplicate", told.sent)
	}
	if !strings.Contains(told.sent[0].text, "the post is up") {
		t.Errorf("the reply that was sent again does not hold what the killed program wrote: %q", told.sent[0].text)
	}
}

// waitForTheHelper reads the helper's output until it says the reply is in the
// log, and fails the test when it never does.
func waitForTheHelper(t *testing.T, printed interface{ Read([]byte) (int, error) }) {
	t.Helper()
	lines := bufio.NewScanner(printed)
	for lines.Scan() {
		if strings.Contains(lines.Text(), helperReady) {
			return
		}
	}
	t.Fatalf("the helper never said that it had written the reply")
}

func TestABackupAndARestoreBringBackARealDatabaseTheVaultAndTheProfile(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	events := writeSomeEvents(t, home.DatabaseFile(), 50)

	archive, err := reliability.Backup(context.Background(), reliability.BackupSettings{
		Home:  home,
		Clock: testkit.NewFakeClock(startOfTime),
	})
	if err != nil {
		t.Fatalf("writing the backup failed: %v", err)
	}

	restored := anEmptyHome(t)
	writeFile(t, restored.VaultKeyFile(), readFile(t, home.VaultKeyFile()), contract.SecretFileMode)
	if err := reliability.Restore(context.Background(), reliability.RestoreSettings{Home: restored, Archive: archive}); err != nil {
		t.Fatalf("putting the backup back failed: %v", err)
	}

	// The database is compared event by event rather than byte by byte, because
	// the copy in the archive is written by SQLite's own VACUUM INTO, which
	// packs the file; everything in it is the same and its bytes are not.
	if brought := readSomeEvents(t, restored.DatabaseFile()); brought != events {
		t.Errorf("the restored database holds:\n%s\nwant:\n%s", brought, events)
	}
	if readFile(t, restored.VaultFile()) != readFile(t, home.VaultFile()) {
		t.Errorf("the vault did not come back the same")
	}
	profile := filepath.Join("default", "Preferences")
	if readFile(t, filepath.Join(restored.BrowserFolder(), profile)) != readFile(t, filepath.Join(home.BrowserFolder(), profile)) {
		t.Errorf("the browser profile did not come back the same")
	}
}

func TestADamagedDatabaseIsMovedAsideAndTheNewestBackupPutBack(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	events := writeSomeEvents(t, home.DatabaseFile(), 200)
	archive, err := reliability.Backup(context.Background(), reliability.BackupSettings{
		Home:  home,
		Clock: testkit.NewFakeClock(startOfTime),
	})
	if err != nil {
		t.Fatalf("writing the backup failed: %v", err)
	}
	told := &sender{}
	guard, err := reliability.New(reliability.Settings{
		Home:  home,
		Clock: testkit.NewFakeClock(startOfTime),
		Store: testkit.NewFakeStore(),
		Caps:  contract.DefaultConfig().Caps,
		Send:  told.send,
	})
	if err != nil {
		t.Fatalf("building the guard failed: %v", err)
	}
	aNewLife(t, home)
	if _, err := guard.Start(context.Background()); err != nil {
		t.Fatalf("the first start failed: %v", err)
	}
	overwriteTheMiddle(t, home.DatabaseFile())

	// The sentinel is still there, because nothing marked a clean exit, so the
	// recovery of this new life is the one that looks at the database.
	aNewLife(t, home)
	found, err := guard.Start(context.Background())
	if err != nil {
		t.Fatalf("the start after the damage failed: %v", err)
	}

	if found.DatabaseMovedTo == "" {
		t.Fatalf("the damaged database was left where the agent would open it: %+v", found)
	}
	if found.RestoredFrom != archive {
		t.Fatalf("the database was put back from %q, want the backup %s", found.RestoredFrom, archive)
	}
	if _, err := os.Stat(found.DatabaseMovedTo); err != nil {
		t.Errorf("the damaged database was thrown away rather than kept: %v", err)
	}
	if brought := readSomeEvents(t, home.DatabaseFile()); brought != events {
		t.Errorf("the database put back holds:\n%s\nwant what the backup held:\n%s", brought, events)
	}
	if len(told.sent) == 0 || !strings.Contains(told.sent[0].text, found.DatabaseMovedTo) {
		t.Errorf("the user was told %q, want a message naming the file that was moved aside", told.sent)
	}
}

func TestSomethingThatIsNotADatabaseFailsTheCheck(t *testing.T) {
	folder := t.TempDir()
	rubbish := filepath.Join(folder, "coeus.db")
	writeFile(t, rubbish, "these are not the bytes you are looking for", contract.SecretFileMode)

	if err := reliability.CheckDatabase(context.Background(), rubbish); err == nil {
		t.Errorf("a file that is not a database passed the check")
	}
	if err := reliability.CheckDatabase(context.Background(), filepath.Join(folder, "nothing-like-this.db")); err != nil {
		t.Errorf("a home with no database yet failed the check: %v", err)
	}
}

func TestABackupOfADamagedDatabaseSaysSoRatherThanWritingHalfAnArchive(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	writeSomeEvents(t, home.DatabaseFile(), 200)
	overwriteTheMiddle(t, home.DatabaseFile())

	_, err := reliability.Backup(context.Background(), reliability.BackupSettings{
		Home:  home,
		Clock: testkit.NewFakeClock(startOfTime),
	})

	if err == nil {
		t.Fatalf("a damaged database was copied into an archive without a word")
	}
	left, err := os.ReadDir(home.BackupsFolder())
	if err != nil {
		t.Fatalf("reading the backups folder failed: %v", err)
	}
	if len(left) != 0 {
		t.Errorf("the backups folder holds %d files after a backup that failed, want none", len(left))
	}
}

func TestADamagedDatabaseWithNoBackupToPutBackStillLeavesTheAgentServing(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	writeSomeEvents(t, home.DatabaseFile(), 200)
	told := &sender{}
	guard, err := reliability.New(reliability.Settings{
		Home:  home,
		Clock: testkit.NewFakeClock(startOfTime),
		Store: testkit.NewFakeStore(),
		Caps:  contract.DefaultConfig().Caps,
		Send:  told.send,
	})
	if err != nil {
		t.Fatalf("building the guard failed: %v", err)
	}
	aNewLife(t, home)
	if _, err := guard.Start(context.Background()); err != nil {
		t.Fatalf("the first start failed: %v", err)
	}
	overwriteTheMiddle(t, home.DatabaseFile())

	aNewLife(t, home)
	found, err := guard.Start(context.Background())

	if err != nil {
		t.Fatalf("the start after the damage failed, and the agent must still come up: %v", err)
	}
	if found.DatabaseMovedTo == "" || found.RestoredFrom != "" {
		t.Errorf("the start came back as %+v, want the database moved aside and nothing put back", found)
	}
	if len(told.sent) == 0 || !strings.Contains(told.sent[0].text, "empty") {
		t.Errorf("the user was told %q, want a message saying it is starting with an empty database", told.sent)
	}
}

// writeSomeEvents fills a real event log and returns what it holds, as text, so
// that a restore can be compared with it.
func writeSomeEvents(t *testing.T, path string, howMany int) string {
	t.Helper()
	opened, err := log.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("opening the event log at %s failed: %v", path, err)
	}
	for number := range howMany {
		body, err := json.Marshal(map[string]any{"text": fmt.Sprintf("event number %d", number)})
		if err != nil {
			t.Fatalf("building the event failed: %v", err)
		}
		event := contract.Event{
			Occurred: startOfTime.Add(time.Duration(number) * time.Second),
			TaskID:   "17",
			Kind:     contract.EventMessage,
			Body:     body,
		}
		if _, err := opened.Append(context.Background(), event); err != nil {
			t.Fatalf("writing event %d failed: %v", number, err)
		}
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("closing the event log failed: %v", err)
	}
	return readSomeEvents(t, path)
}

// readSomeEvents reads a real event log back as text.
func readSomeEvents(t *testing.T, path string) string {
	t.Helper()
	opened := aRealLog(t, path)
	events, err := opened.ByTask(context.Background(), "17")
	if err != nil {
		t.Fatalf("reading the events back failed: %v", err)
	}
	lines := []string{}
	for _, event := range events {
		lines = append(lines, fmt.Sprintf("%d %s %s", event.Sequence, event.Kind, event.Body))
	}
	return strings.Join(lines, "\n")
}

// overwriteTheMiddle writes rubbish over the middle of a file, which is what a
// disk going bad does to a database.
func overwriteTheMiddle(t *testing.T, path string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY, contract.SecretFileMode)
	if err != nil {
		t.Fatalf("opening %s to damage it failed: %v", path, err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		t.Fatalf("reading the size of %s failed: %v", path, err)
	}
	rubbish := make([]byte, 8192)
	for at := range rubbish {
		rubbish[at] = 0xEE
	}
	if _, err := file.WriteAt(rubbish, info.Size()/2); err != nil {
		t.Fatalf("writing over the middle of %s failed: %v", path, err)
	}
}

func TestTheEventsWrittenAfterARecoveryGoIntoTheDatabaseTheAgentIsUsing(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	writeSomeEvents(t, home.DatabaseFile(), 200)
	if _, err := reliability.Backup(context.Background(), reliability.BackupSettings{
		Home:  home,
		Clock: testkit.NewFakeClock(startOfTime),
	}); err != nil {
		t.Fatalf("writing the backup failed: %v", err)
	}

	// The life before this one was killed, so its sentinel is still there, and
	// the database went bad while it was running.
	aNewLife(t, home)
	overwriteTheMiddle(t, home.DatabaseFile())

	// This is the order the program opens things in: the recovery first, and
	// then the event log on the path the recovery hands back.
	databaseFile := aNewLife(t, home)
	opened := aRealLog(t, databaseFile)
	told := &sender{}
	guard, err := reliability.New(reliability.Settings{
		Home:  home,
		Clock: testkit.NewFakeClock(startOfTime),
		Store: opened,
		Caps:  contract.DefaultConfig().Caps,
		Send:  told.send,
	})
	if err != nil {
		t.Fatalf("building the guard failed: %v", err)
	}
	found, err := guard.Start(context.Background())
	if err != nil {
		t.Fatalf("starting after the damage failed: %v", err)
	}
	if _, err := opened.Append(context.Background(), contract.Event{
		Occurred: startOfTime.Add(time.Hour),
		TaskID:   "17",
		Kind:     contract.EventMessage,
		Body:     []byte(`{"text":"the first thing the agent did after the recovery"}`),
	}); err != nil {
		t.Fatalf("writing the first event after the recovery failed: %v", err)
	}

	if found.DatabaseMovedTo == "" {
		t.Fatalf("the damaged database was left where the agent would open it: %+v", found)
	}
	if !strings.Contains(readSomeEvents(t, home.DatabaseFile()), "after the recovery") {
		t.Errorf("the database the agent is using does not hold the event written after the recovery, so the event went into the file that was moved aside and the user has been told to ignore it")
	}
	if _, err := os.Stat(found.DatabaseMovedTo); err != nil {
		t.Errorf("the damaged database was thrown away rather than kept: %v", err)
	}
}
