package job

import (
	"testing"
	"time"
)

// TestEveryBoundIsTheNumberItIsMeantToBe writes each bound out as the literal it
// is, with the sentence saying why it is that number. Every test around it
// measures against the constant, so it moves whenever the constant moves and
// says nothing about whether the number is still the right one; this test is
// what goes red when somebody changes a bound without meaning to.
func TestEveryBoundIsTheNumberItIsMeantToBe(t *testing.T) {
	for _, bound := range []struct {
		name string
		is   int
		want int
		why  string
	}{
		{"MaxJobs", MaxJobs, 500,
			"a job is a month of work, so a machine holding more than five hundred has a job that is making jobs"},
		{"MaxTasksPerJob", MaxTasksPerJob, 200,
			"two hundred tasks is where a job record stops fitting in the one to three thousand tokens the design gives it"},
		{"NotepadBytes", NotepadBytes, 16384,
			"sixteen kilobytes is a notepad a job carries between its tasks without filling the model's window"},
		{"MaxIncidents", MaxIncidents, 50,
			"fifty distinct failures is more than any job the user will want to keep running has met"},
		{"FailuresThatPause", FailuresThatPause, 3,
			"three failures in a row are enough to fetch the person who is watching a plain job"},
		{"FailuresThatSwitchOff", FailuresThatSwitchOff, 10,
			"ten failed nights are a broken schedule rather than a bad afternoon"},
		{"maxConnections", maxConnections, 4,
			"a claim is one short statement, so four connections to the database file are plenty"},
		{"signatureLength", signatureLength, 12,
			"twelve letters of a hash are short enough to read out loud and long enough not to collide in fifty incidents"},
		{"errorTextRead", errorTextRead, 200,
			"two hundred letters of an error decide which incident it is, however long the tail it printed"},
		{"titleWidth", titleWidth, 100,
			"a hundred letters of a job's ask fit the column a listing is read down"},
		{"maxWrappingShells", maxWrappingShells, 3,
			"three shells wrapped inside one another is more than any command a person writes needs"},
	} {
		if bound.is != bound.want {
			t.Errorf("%s is %d, want %d, because %s", bound.name, bound.is, bound.want, bound.why)
		}
	}

	for _, bound := range []struct {
		name string
		is   time.Duration
		want time.Duration
		why  string
	}{
		{"TaskBudget", TaskBudget, time.Hour,
			"an hour is the hard stop on one task, after which its claim is another process's to take"},
		{"TimerClamp", TimerClamp, 60 * time.Second,
			"a minute is the longest the store sleeps, so a job changed while the agent waits is picked up within the minute"},
		{"RestBetweenWaits", RestBetweenWaits, time.Second,
			"a second between two waits is what stops a driver that cannot take the work yet from spinning"},
		{"shortestInterval", shortestInterval, time.Minute,
			"a task takes minutes, so a schedule asking for one every second is asking for work that cannot be done"},
	} {
		if bound.is != bound.want {
			t.Errorf("%s is %s, want %s, because %s", bound.name, bound.is, bound.want, bound.why)
		}
	}
}

// TestTheBackoffTableIsTheOneTheDesignNames writes out the five steps a failed
// tick climbs through, because the table is a bound as much as any number is.
func TestTheBackoffTableIsTheOneTheDesignNames(t *testing.T) {
	want := []time.Duration{30 * time.Second, 60 * time.Second, 5 * time.Minute, 15 * time.Minute, 60 * time.Minute}

	if len(backoffSteps) != len(want) {
		t.Fatalf("the backoff table has %d steps, want the five of thirty seconds, a minute, five, fifteen and sixty", len(backoffSteps))
	}
	for at, step := range want {
		if backoffSteps[at] != step {
			t.Errorf("backoff step %d is %s, want %s", at+1, backoffSteps[at], step)
		}
	}
}
