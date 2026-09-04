package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
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
