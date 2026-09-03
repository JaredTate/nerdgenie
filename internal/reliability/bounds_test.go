package reliability

import (
	"testing"
	"time"
)

// These tests write the number itself, and one sentence saying why it is that
// number. Every other test in this package measures against the constant it is
// testing, so the number could be changed to anything and the suite would stay
// green; the wave 6 review changed fifteen of the seventeen bounds here and
// nothing went red. A literal is the only assertion a mutation cannot follow.

func TestEveryCountedBoundIsTheNumberItWasChosenToBe(t *testing.T) {
	for _, one := range []struct {
		name string
		is   int
		want int
		why  string
	}{
		{"RestartLimit", RestartLimit, 3,
			"two starts after a crash is an operator restarting the program and three inside the window is a loop"},
		{"MaxLeases", MaxLeases, 512,
			"a machine running this many turns at once is in trouble already, and refusing the next one says so"},
		{"MaxDeliveryAttempts", MaxDeliveryAttempts, 3,
			"three tries is enough for a channel that is coming back and few enough that a reply nobody can deliver stops"},
		{"MaxUndeliveredReplies", MaxUndeliveredReplies, 1000,
			"a log with more than a thousand replies waiting is a log to read by hand rather than to hold in memory"},
		{"KeptBackups", KeptBackups, 7,
			"a week of nightly archives, so that a fault noticed on Monday can be undone from the Sunday before"},
		{"MaxArchiveEntries", MaxArchiveEntries, 100000,
			"a backup holds a database, a vault, and a browser profile, and a browser profile is thousands of small files"},
		{"maxCheckLines", maxCheckLines, 5,
			"a file broken in a hundred places is as broken as a file broken in five, and the rest is not worth reading"},
	} {
		if one.is != one.want {
			t.Errorf("%s is %d, want %d, because %s", one.name, one.is, one.want, one.why)
		}
	}
}

func TestEveryLengthOfTimeIsTheOneItWasChosenToBe(t *testing.T) {
	for _, one := range []struct {
		name string
		is   time.Duration
		want time.Duration
		why  string
	}{
		{"RestartWindow", RestartWindow, 5 * time.Minute,
			"starts further apart than five minutes belong to a crash loop that is over"},
		{"QuietPeriod", QuietPeriod, 30 * time.Minute,
			"half an hour is long enough for whatever was killing the program to be over and short enough that a machine nobody is watching heals"},
		{"LeaseWait", LeaseWait, 5 * time.Second,
			"five seconds is long enough for a short turn to finish and short enough that the second message is answered rather than left"},
		{"leaseStep", leaseStep, 25 * time.Millisecond,
			"a turn that finishes quickly is not made to wait, and looking this often costs nothing"},
		{"DeliveryLifetime", DeliveryLifetime, 24 * time.Hour,
			"after a day the answer is stale, and a message out of nowhere confuses more than it helps"},
		{"DrainExpiry", DrainExpiry, 30 * time.Minute,
			"a marker still there half an hour later belongs to a writer that never came back, and must not park the agent forever"},
	} {
		if one.is != one.want {
			t.Errorf("%s is %s, want %s, because %s", one.name, one.is, one.want, one.why)
		}
	}
}

func TestEveryByteCapIsTheNumberItWasChosenToBe(t *testing.T) {
	for _, one := range []struct {
		name string
		is   int64
		want int64
		why  string
	}{
		{"MaxArchiveBytes", MaxArchiveBytes, 2 << 30,
			"two gigabytes is more than a home folder holds and small enough that an archive claiming more is refused before it is unpacked"},
		{"maxKeyFileBytes", maxKeyFileBytes, 4096,
			"an age private key is one line, so a longer file is not one of ours"},
		{"maxStateFileBytes", maxStateFileBytes, 1 << 20,
			"every one of these files holds a handful of times and names, so anything longer is not one of ours"},
	} {
		if one.is != one.want {
			t.Errorf("%s is %d, want %d, because %s", one.name, one.is, one.want, one.why)
		}
	}
}
