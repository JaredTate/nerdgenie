package reliability_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/reliability"
	"github.com/JaredTate/coeus/internal/testkit"
)

// aBreaker builds a breaker on a temporary home and the clock the test moves.
func aBreaker(t *testing.T) (*reliability.Breaker, *testkit.FakeClock, contract.Home) {
	t.Helper()
	home := testkit.NewTempHome(t)
	clock := testkit.NewFakeClock(startOfTime)
	return reliability.NewBreaker(home, clock), clock, home
}

// tripIt records unclean starts until the breaker says it has tripped, failing
// the test when it never does.
func tripIt(t *testing.T, breaker *reliability.Breaker) {
	t.Helper()
	for range reliability.RestartLimit {
		tripped, err := breaker.RecordUncleanStart()
		if err != nil {
			t.Fatalf("recording an unclean start failed: %v", err)
		}
		if tripped {
			return
		}
	}
	t.Fatalf("the breaker did not trip after %d unclean starts", reliability.RestartLimit)
}

func TestTheBreakerTripsAtTheLimitOfUncleanStarts(t *testing.T) {
	breaker, clock, _ := aBreaker(t)

	for start := 1; start < reliability.RestartLimit; start++ {
		tripped, err := breaker.RecordUncleanStart()
		if err != nil {
			t.Fatalf("recording unclean start %d failed: %v", start, err)
		}
		if tripped {
			t.Fatalf("the breaker tripped after %d unclean starts, and the limit is %d", start, reliability.RestartLimit)
		}
		clock.Advance(10 * time.Second)
	}

	tripped, err := breaker.RecordUncleanStart()
	if err != nil {
		t.Fatalf("recording the last unclean start failed: %v", err)
	}
	if !tripped {
		t.Fatalf("the breaker has not tripped after %d unclean starts", reliability.RestartLimit)
	}
	if nowTripped, err := breaker.Tripped(); err != nil || !nowTripped {
		t.Errorf("the tripped breaker reports %v, %v, want it tripped", nowTripped, err)
	}
}

func TestUncleanStartsFurtherApartThanTheWindowNeverTrip(t *testing.T) {
	breaker, clock, _ := aBreaker(t)

	for range reliability.RestartLimit + 2 {
		tripped, err := breaker.RecordUncleanStart()
		if err != nil {
			t.Fatalf("recording an unclean start failed: %v", err)
		}
		if tripped {
			t.Fatalf("the breaker tripped on starts %s apart, which is not a crash loop", reliability.RestartWindow)
		}
		clock.Advance(reliability.RestartWindow + time.Minute)
	}
}

func TestTheBreakerClearsItselfAfterTheQuietPeriod(t *testing.T) {
	breaker, clock, home := aBreaker(t)
	tripIt(t, breaker)

	clock.Advance(reliability.QuietPeriod - time.Minute)
	if tripped, err := breaker.Tripped(); err != nil || !tripped {
		t.Fatalf("the breaker cleared itself before the quiet period was over: %v, %v", tripped, err)
	}

	clock.Advance(2 * time.Minute)

	tripped, err := breaker.Tripped()
	if err != nil {
		t.Fatalf("reading the breaker after the quiet period failed: %v", err)
	}
	if tripped {
		t.Errorf("the breaker is still tripped %s after it tripped", reliability.QuietPeriod)
	}
	if _, err := os.Stat(filepath.Join(home.RunFolder(), reliability.BreakerFileName)); !os.IsNotExist(err) {
		t.Errorf("the breaker left its state file behind after clearing itself: %v", err)
	}
}

func TestTheBreakerSaysOneThingToTheUserAndThenNothing(t *testing.T) {
	breaker, _, _ := aBreaker(t)
	if said, err := breaker.Announce(); err != nil || said != "" {
		t.Fatalf("a breaker that has not tripped wants to say %q, %v, want nothing", said, err)
	}

	tripIt(t, breaker)

	said, err := breaker.Announce()
	if err != nil {
		t.Fatalf("asking the tripped breaker what to say failed: %v", err)
	}
	if !strings.Contains(said, "task") {
		t.Errorf("the breaker's message does not say that it will start no task:\n%s", said)
	}
	again, err := breaker.Announce()
	if err != nil {
		t.Fatalf("asking the breaker a second time failed: %v", err)
	}
	if again != "" {
		t.Errorf("the breaker wants to say %q a second time, and the user is told once", again)
	}
}

func TestTheBreakerIsTheSameAcrossTwoRunsOfTheProgram(t *testing.T) {
	home := testkit.NewTempHome(t)
	clock := testkit.NewFakeClock(startOfTime)
	tripIt(t, reliability.NewBreaker(home, clock))

	afterRestart := reliability.NewBreaker(home, clock)

	tripped, err := afterRestart.Tripped()
	if err != nil {
		t.Fatalf("reading the breaker in the next run failed: %v", err)
	}
	if !tripped {
		t.Errorf("the breaker forgot that it had tripped when the program started again")
	}
}

func TestACleanExitClearsTheBreaker(t *testing.T) {
	breaker, _, home := aBreaker(t)
	tripIt(t, breaker)

	if err := breaker.Clear(); err != nil {
		t.Fatalf("clearing the breaker failed: %v", err)
	}

	if tripped, err := breaker.Tripped(); err != nil || tripped {
		t.Errorf("the breaker is still tripped after a clean exit: %v, %v", tripped, err)
	}
	if err := breaker.Clear(); err != nil {
		t.Errorf("clearing a breaker that is already clear failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home.RunFolder(), reliability.BreakerFileName)); !os.IsNotExist(err) {
		t.Errorf("the state file is still there after the breaker was cleared: %v", err)
	}
}

func TestADamagedBreakerFileNeverStopsTheProgramFromServing(t *testing.T) {
	breaker, _, home := aBreaker(t)
	path := filepath.Join(home.RunFolder(), reliability.BreakerFileName)
	if err := os.WriteFile(path, []byte("{not json at all"), contract.DataFileMode); err != nil {
		t.Fatalf("writing the damaged state file failed: %v", err)
	}

	tripped, err := breaker.Tripped()
	if err == nil {
		t.Errorf("a damaged state file was read without a word about it")
	}
	if tripped {
		t.Errorf("a damaged state file left the breaker tripped, so a healthy agent would refuse to work")
	}

	if _, err := breaker.RecordUncleanStart(); err != nil {
		t.Fatalf("recording a start over a damaged state file failed: %v", err)
	}
	if tripped, err := breaker.Tripped(); err != nil || tripped {
		t.Errorf("the damaged file was not replaced by a good one: %v, %v", tripped, err)
	}
}

func TestTheBreakerSaysWhenItCannotWriteItsState(t *testing.T) {
	home := contract.NewHome(filepath.Join(t.TempDir(), "cannot-be-written-in"))
	if err := os.Mkdir(home.Root, 0o500); err != nil {
		t.Fatalf("making the read-only home failed: %v", err)
	}
	breaker := reliability.NewBreaker(home, testkit.NewFakeClock(startOfTime))

	_, err := breaker.RecordUncleanStart()
	if err == nil {
		t.Fatalf("the breaker said nothing when its state could not be written")
	}
	if !strings.Contains(err.Error(), reliability.BreakerFileName) {
		t.Errorf("the failure does not name the file it could not write: %v", err)
	}
}
