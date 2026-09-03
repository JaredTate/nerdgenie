package update

import (
	"testing"
	"time"
)

// The counts and sizes this package promises, written out as the numbers they
// are. A bound asserted against the constant that holds it is not pinned at all,
// because the constant and the test move together and nothing fails. These are
// the literals, so changing one of them fails here and the change is deliberate.
func TestEveryCountAndSizeOfAnUpdateIsTheNumberItIsMeantToBe(t *testing.T) {
	bounds := []struct {
		name string
		held int64
		want int64
	}{
		{"the installed releases that are kept", KeptReleases, 3},
		{"the most readiness questions one wait asks", maxReadyAsks, 1000},
		{"the messages the agent may send before its answer", maxReadyLines, 64},
		{"the most one readiness answer may be, in bytes", maxReadyBytes, 1 << 20},
		{"the most one release archive may be, in bytes", MaxDownloadBytes, 256 << 20},
		{"the most one manifest may be, in bytes", MaxManifestBytes, 64 << 10},
		{"the entries one release archive may hold", int64(archiveEntryCap), 20000},
		{"the bytes one release may write to disk", unpackedByteCap, 512 << 20},
		{"the words of what the service manager said that are kept", maxServiceOutput, 4096},
		{"the dotted numbers a version is read as", maxVersionParts, 3},
	}
	for _, bound := range bounds {
		if bound.held != bound.want {
			t.Errorf("%s is %d, want %d; if the change is meant, change this test with it",
				bound.name, bound.held, bound.want)
		}
	}
}

// The waits this package promises. The first of them is the sixty seconds the
// whole rollback rule is named after: a new version that has not answered by
// then is one the link goes back off.
func TestEveryWaitOfAnUpdateIsTheTimeItIsMeantToBe(t *testing.T) {
	waits := []struct {
		name string
		held time.Duration
		want time.Duration
	}{
		{"the deadline a new version has to come up in", ReadyDeadline, 60 * time.Second},
		{"how often the new version is asked whether it is up", ReadyPoll, 2 * time.Second},
		{"how long opening the agent's socket may take", dialWait, 2 * time.Second},
		{"how long the agent has to answer one readiness question", answerWait, 5 * time.Second},
		{"how long reading the manifest may take", manifestWait, time.Minute},
		{"how long fetching one archive may take", downloadWait, 15 * time.Minute},
		{"how long one systemctl call may take", serviceWait, 30 * time.Second},
		{"how long the migration step may take", migrateWait, 10 * time.Minute},
	}
	for _, wait := range waits {
		if wait.held != wait.want {
			t.Errorf("%s is %s, want %s; if the change is meant, change this test with it",
				wait.name, wait.held, wait.want)
		}
		if wait.held <= 0 {
			t.Errorf("%s is %s, and every wait of an update is bounded above zero", wait.name, wait.held)
		}
	}
}
