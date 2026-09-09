package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

func TestTheTaskHasEndedReadsTheRecordLine(t *testing.T) {
	for _, ending := range []string{"done", "failed", "stopped", "waiting"} {
		fields := map[string]string{contract.StatusFieldRecordLine: "task 7 " + ending + " something"}
		if !theTaskHasEnded(fields) {
			t.Errorf("a record line that says %q was not read as ended", ending)
		}
	}
	if theTaskHasEnded(map[string]string{contract.StatusFieldRecordLine: "task 7 running the shell"}) {
		t.Error("a running task was read as ended, and --wait would stop too early")
	}
	if theTaskHasEnded(map[string]string{}) {
		t.Error("an empty record line was read as ended")
	}
}

func TestTroubleInSaysWhatWentWrongOrThatItDidNotSay(t *testing.T) {
	said := troubleIn(contract.SocketEnvelope{Text: "the tool failed", Reason: "no network"})
	if !strings.Contains(said, "the tool failed") || !strings.Contains(said, "no network") {
		t.Errorf("the trouble is reported as %q, want both the text and the reason", said)
	}
	if empty := troubleIn(contract.SocketEnvelope{}); empty == "" {
		t.Error("an error with no words gave back nothing, and a person needs to be told something")
	}
}

func TestWhyTheReadingStoppedTurnsTheEndIntoASentence(t *testing.T) {
	chosen := runOptions{timeout: 3 * time.Second}
	if err := whyTheReadingStopped(nil, chosen, true); err != nil {
		t.Errorf("an exchange that got a reply reported %v, want nothing", err)
	}
	if err := whyTheReadingStopped(os.ErrDeadlineExceeded, chosen, false); err == nil || !strings.Contains(err.Error(), chosen.timeout.String()) {
		t.Errorf("a deadline with no reply reported %v, want a line naming the timeout", err)
	}
	if err := whyTheReadingStopped(errors.New("the link to the agent was reset by the machine"), chosen, false); err == nil || !strings.Contains(err.Error(), "reset by the machine") {
		t.Errorf("a broken link reported %v, want it to name what went wrong", err)
	}
}

func TestWritingTheLinesWorthSeeingPrintsRecordAndToolLines(t *testing.T) {
	var problems bytes.Buffer
	writeTheLinesWorthSeeing(map[string]string{
		contract.StatusFieldRecordLine: "task 7 started the work",
		contract.StatusFieldToolLine:   "running the shell tool",
	}, &problems)
	if !strings.Contains(problems.String(), "started the work") || !strings.Contains(problems.String(), "shell tool") {
		t.Errorf("the lines worth seeing printed %q, want both the record and the tool line", problems.String())
	}

	var quiet bytes.Buffer
	writeTheLinesWorthSeeing(map[string]string{}, &quiet)
	if quiet.String() != "" {
		t.Errorf("empty fields still printed %q", quiet.String())
	}
}

func TestWasItGivenSaysWhetherTheFlagWasThere(t *testing.T) {
	if wasItGiven(true) != "given" || wasItGiven(false) != "not given" {
		t.Error("wasItGiven does not say plainly whether --yes was there")
	}
}

// TestTheTaskHasEndedDoesNotStopOnAJobsOwnTaskEnding locks the fix for a
// workorder: a job's per-task record line ("job N task M done") is not read as
// the end, because the job has more tasks or closes with a reply theJobHasEnded
// reads. Before this, --wait on a workorder exited after the job's first task.
func TestTheTaskHasEndedDoesNotStopOnAJobsOwnTaskEnding(t *testing.T) {
	for _, ending := range []string{"done", "failed", "stopped", "waiting"} {
		line := "job 4 task 1 " + ending + " · scaffold the project"
		if theTaskHasEnded(map[string]string{contract.StatusFieldRecordLine: line}) {
			t.Errorf("a job's own task line %q was read as the whole task ending, and --wait would stop after the job's first task", line)
		}
	}
	// A plain task's ending is unchanged: it is the whole of the work.
	if !theTaskHasEnded(map[string]string{contract.StatusFieldRecordLine: "task 7 done · count the jars"}) {
		t.Error("a plain task that is done was not read as ended")
	}
}

// TestTheJobHasEndedReadsAJobsClosingReply: --wait on a workorder waits until
// the job's own closing reply, and reads it in every shape the job ends in,
// but not the replies a job sends while it is still going.
func TestTheJobHasEndedReadsAJobsClosingReply(t *testing.T) {
	over := []string{
		"Job 4 is finished: every one of its 3 tasks is done in 41m.",
		"Job 4 has run every task, and its done list is not proven yet. line 2 is not proved",
		"Job 4 is paused on task t2. Your next message picks this task up; type /clear first to set it aside.",
		"Job 4 is waiting on task t2 for your answer.",
	}
	for _, reply := range over {
		if !theJobHasEnded(reply) {
			t.Errorf("a job's closing reply %q was not read as the job ending, so --wait would hang past it", reply)
		}
	}
	// A put-down reply carries the task's report first, with the closing line
	// under it, and must still be read.
	if !theJobHasEnded("It is done. What changed: the sky.\nJob 4 is paused on task t2. Your next message picks this task up.") {
		t.Error("a job's closing line under a task report was not read as the job ending")
	}
	stillGoing := []string{
		"Job 4 keeps working task t2 itself on a fresh window rather than waiting for anyone; it will not stop until the job is done.",
		"Job 4 picks task t2 up itself, once, on a fresh window; if it stops the same way again, the job sets it aside and goes on.",
		"Task t2 is set aside after stopping twice; job 4 goes on with the next task and comes back to t2 before its last task.",
		"It is done. What changed: nothing.\nJob 4, report j4.1: 1 of 3 tasks done.",
		"Made job 4 from the work order, with 3 tasks, t1 to t3. Task t1 starts now.",
		"there are four jars on the shelf",
	}
	for _, reply := range stillGoing {
		if theJobHasEnded(reply) {
			t.Errorf("a reply the job sends while it is still going, %q, was read as the job ending, so --wait would stop too early", reply)
		}
	}
}
