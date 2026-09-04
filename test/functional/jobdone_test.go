// The whole-program test for a job finishing. A person's ask became a job with
// two tasks, both ran, and the job could never close, because closing one ran
// the done-check on a done list nothing lets the model write. Now the store
// writes one done line per task when the last task finishes, so the job is
// listed as done, its final report reaches the person, and the status stops
// counting it among the jobs still waiting to run.
package functional

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// theJobsLeftWaitingOnceTheModelsIsDone is what the status counts once the
// model's job is done: the nightly self-check, which every fresh home
// registers and which runs for as long as the agent does.
const theJobsLeftWaitingOnceTheModelsIsDone = "1"

func TestAJobMadeThroughTheToolReachesDoneAndStopsCountingAsWaiting(t *testing.T) {
	agent := startTheAgentWorkingIn(t, aJobOfTwoTasksMadeByTheModel)

	screen := agent.attach(t)
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketMessage, Text: theAskThatIsAJob})

	// While the job's tasks run the status counts two jobs waiting: the nightly
	// self-check and the model's.
	screen.waitForStatusWhere(t, theTimeAJobsFirstTaskIsGiven, func(fields map[string]string) bool {
		return fields[contract.StatusFieldJob] == theJobTheModelMakes && fields[contract.StatusFieldJobs] == "2"
	})
	screen.waitForReplySaying(t, "Job "+theJobTheModelMakes+", report j"+theJobTheModelMakes+".2: 2 of 2 tasks done", 90*time.Second)
	final := screen.waitForReplySaying(t, theWordsOfAFinishedJob, 90*time.Second)
	if !strings.Contains(final.Text, "every one of its 2 tasks is done") {
		t.Errorf("the final report reads %q, want it to say every one of the job's two tasks is done", final.Text)
	}

	screen.waitForStatusWhere(t, 30*time.Second, func(fields map[string]string) bool {
		return fields[contract.StatusFieldJob] == "" && fields[contract.StatusFieldJobs] == theJobsLeftWaitingOnceTheModelsIsDone
	})
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "/jobs"})
	listing := screen.waitForReplySaying(t, "Jobs, oldest first", 30*time.Second)
	line := theLineAbout(t, listing.Text, theJobTheModelMakes)
	if !strings.Contains(line, string(contract.JobDone)) || !strings.Contains(line, "2 of 2 tasks done") {
		t.Errorf("after its last task the job lists as %q, want it done with both tasks done", line)
	}
	screen.send(t, contract.SocketEnvelope{Type: contract.SocketCommand, Text: "/jobs " + theJobTheModelMakes})
	held := screen.waitForReplySaying(t, "# job "+theJobTheModelMakes, 30*time.Second)
	for _, words := range []string{"[x] post the tweet -> j" + theJobTheModelMakes + ".1", "[x] write the summary for the user -> j" + theJobTheModelMakes + ".2"} {
		if !strings.Contains(held.Text, words) {
			t.Errorf("the finished job's record reads:\n%s\nwant the done line %q, one per task and proved by its report", held.Text, words)
		}
	}
}
