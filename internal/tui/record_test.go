package tui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// aRecordLine is a status carrying one line about the latest change to the
// record, which is what the program sends when a task or a job is created,
// started, finished, or fails.
func aRecordLine(line string) contract.SocketEnvelope {
	return contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldRecordLine: line,
	}}
}

// pillTexts is the text of every pill in the transcript, in the order they were
// put there.
func pillTexts(screen *Screen) []string {
	said := []string{}
	for _, one := range screen.blocks {
		if one.kind == blockTool {
			said = append(said, one.text)
		}
	}
	return said
}

func TestARecordLineIsShownAsOnePill(t *testing.T) {
	screen, _ := screenWithLink()
	screen.Update(linkMessage{up: true})
	send(screen, aRecordLine("task 3 started · Build the game"))

	if said := pillTexts(screen); len(said) != 1 || said[0] != "task 3 started · Build the game" {
		t.Fatalf("a record line drew the pills %q, and it is one pill saying what changed", said)
	}
	drawn := plainText(strings.Join(screen.blockLines(screen.blocks[0]), "\n"))
	if !strings.Contains(drawn, string(toolArrowGlyph)+" task 3 started · Build the game") {
		t.Errorf("the record line is drawn as %q, and it is drawn exactly as a tool line is", drawn)
	}
}

func TestTheSameRecordLineTwiceDrawsOnePill(t *testing.T) {
	screen, _ := screenWithLink()
	screen.Update(linkMessage{up: true})
	for range 4 {
		send(screen, aRecordLine("task 3 started · Build the game"))
	}

	if said := pillTexts(screen); len(said) != 1 {
		t.Errorf("the same record line on four heartbeats drew the pills %q, and the record changed once", said)
	}
	if frame := plainText(screen.frame()); strings.Contains(frame, string(repeatGlyph)) {
		t.Errorf("the same record line on four heartbeats is one change, and it was given a count:\n%s", frame)
	}
}

func TestEachChangeToTheRecordGetsItsOwnPill(t *testing.T) {
	screen, _ := screenWithLink()
	screen.Update(linkMessage{up: true})
	send(screen, aRecordLine("task 3 started · Build the game"))
	send(screen, aRecordLine("task 3 started · Build the game"))
	send(screen, aRecordLine("task 3 finished · Build the game"))
	send(screen, aRecordLine("job 8 created · Post the weekly note"))

	wanted := []string{
		"task 3 started · Build the game",
		"task 3 finished · Build the game",
		"job 8 created · Post the weekly note",
	}
	said := pillTexts(screen)
	if len(said) != len(wanted) {
		t.Fatalf("the record drew the pills %q, and each change gets one: %q", said, wanted)
	}
	for at, one := range wanted {
		if said[at] != one {
			t.Errorf("pill %d says %q, and it should say %q", at+1, said[at], one)
		}
	}
}

func TestARecordLineOfNothingIsNotDrawnAtAll(t *testing.T) {
	screen, _ := screenWithLink()
	screen.Update(linkMessage{up: true})
	send(screen, aRecordLine(""))

	if said := pillTexts(screen); len(said) != 0 {
		t.Errorf("an empty record line drew the pills %q, and there was nothing to say", said)
	}
}

// The lines a live run drew over and over: the same task's ending and the same
// job's task starting, each coming back after the other, until the transcript
// held a column of pills that all said one thing.
const (
	theTaskDoneLine    = "task 1 done · Build a complete Tetris game in the browser"
	theJobTaskLine     = "job 2 task 2 started · Add the seven tetrominoes"
	theTaskStartedLine = "task 1 started · Build a complete Tetris game in the browser"
)

// recordPillCounts is the count on every record pill in the transcript, in the
// order the pills were put there, with zero and one both meaning once.
func recordPillCounts(screen *Screen) []int {
	counts := []int{}
	for _, one := range screen.blocks {
		if one.kind == blockTool && one.fromRecord {
			counts = append(counts, one.repeats)
		}
	}
	return counts
}

func TestARecordLineThatComesBackAfterAnotherIsCountedOnItsPill(t *testing.T) {
	screen := aTrialScreen()
	send(screen, aRecordLine(theTaskDoneLine))
	send(screen, aRecordLine(theJobTaskLine))
	send(screen, aRecordLine(theTaskDoneLine))

	said := pillTexts(screen)
	if len(said) != 2 || said[0] != theTaskDoneLine || said[1] != theJobTaskLine {
		t.Fatalf("a record line that came back after another drew the pills %q, and it should be two pills, the distinct lines in order", said)
	}
	if counts := recordPillCounts(screen); counts[0] != 2 || counts[1] > 1 {
		t.Errorf("the counts on the pills are %v, and the line that came back is counted twice on its own pill and the other not at all", counts)
	}
	frame := plainText(screen.frame())
	if strings.Count(frame, "task 1 done") != 1 || !strings.Contains(frame, "× 2") {
		t.Errorf("the frame should draw the line that came back once with a count of two:\n%s", frame)
	}
}

func TestARecordLineThatKeepsComingBackIsOnePillWithACount(t *testing.T) {
	screen := aTrialScreen()
	for range 3 {
		send(screen, aRecordLine(theTaskDoneLine))
		send(screen, aRecordLine(theJobTaskLine))
	}

	if said := pillTexts(screen); len(said) != 2 {
		t.Fatalf("two record lines taking turns six times drew the pills %q, and each line has one pill", said)
	}
	frame := plainText(screen.frame())
	if strings.Count(frame, "task 1 done") != 1 || strings.Count(frame, "job 2 task 2 started") != 1 {
		t.Errorf("each of the two lines is drawn once:\n%s", frame)
	}
	if strings.Count(frame, "× 3") != 2 {
		t.Errorf("each of the two pills says how many times its line came back, three:\n%s", frame)
	}
}

func TestTheOrdinarySequenceOfRecordLinesDrawsOnePillEach(t *testing.T) {
	screen := aTrialScreen()
	send(screen, aRecordLine(theTaskStartedLine))
	send(screen, aRecordLine(theTaskDoneLine))
	send(screen, aRecordLine(theJobTaskLine))

	wanted := []string{theTaskStartedLine, theTaskDoneLine, theJobTaskLine}
	said := pillTexts(screen)
	if len(said) != len(wanted) {
		t.Fatalf("started, done, and the next task drew the pills %q, and each is a change of its own: %q", said, wanted)
	}
	for at, one := range wanted {
		if said[at] != one {
			t.Errorf("pill %d says %q, and it should say %q", at+1, said[at], one)
		}
	}
	if frame := plainText(screen.frame()); strings.Contains(frame, string(repeatGlyph)) {
		t.Errorf("a count is drawn where no record line came back:\n%s", frame)
	}
}

func TestTheSameRecordLineAroundAToolCallIsOneChange(t *testing.T) {
	screen := aTrialScreen()
	send(screen, aRecordLine(theTaskDoneLine))
	send(screen, aToolLine("▸ read notes.md"))
	send(screen, aToolLine("▸ read notes.md · r4 read: 12 lines"))
	send(screen, aRecordLine(theTaskDoneLine))

	if counts := recordPillCounts(screen); len(counts) != 1 || counts[0] > 1 {
		t.Errorf("the record pills are counted %v, and the same line on the heartbeats around a tool call is one change, so it is one pill with no count", counts)
	}
}

func TestToolCallsBetweenRecordLinesDoNotCountAgainstTheLookBack(t *testing.T) {
	screen := aTrialScreen()
	send(screen, aRecordLine(theTaskDoneLine))
	for step := 1; step < recentRecordPills; step++ {
		send(screen, aRecordLine("job 2 task "+strconv.Itoa(step+1)+" started · Add the seven tetrominoes"))
		send(screen, aToolLine("▸ read notes"+strconv.Itoa(step)+".md"))
		send(screen, aToolLine("▸ read notes"+strconv.Itoa(step)+".md · r"+strconv.Itoa(step)+" read: 12 lines"))
	}
	send(screen, aRecordLine(theTaskDoneLine))

	if said := pillTexts(screen); len(said) != 1+2*(recentRecordPills-1) || said[0] != theTaskDoneLine {
		t.Errorf("a line that came back after %d record lines and as many tool calls drew the pills %q, and the look-back is over record pills, so the calls between do not push it out", recentRecordPills-1, said)
	}
	if counts := recordPillCounts(screen); len(counts) == 0 || counts[0] != 2 {
		t.Errorf("the counts on the record pills are %v, and the first pill is the one counted twice", counts)
	}
}

func TestARecordLineThatComesBackPastTheLookBackIsANewPill(t *testing.T) {
	screen := aTrialScreen()
	send(screen, aRecordLine(theTaskDoneLine))
	for step := 1; step <= recentRecordPills; step++ {
		send(screen, aRecordLine("job 2 task "+strconv.Itoa(step+1)+" started · Add the seven tetrominoes"))
	}
	send(screen, aRecordLine(theTaskDoneLine))

	said := pillTexts(screen)
	if len(said) != recentRecordPills+2 || said[0] != theTaskDoneLine || said[len(said)-1] != theTaskDoneLine {
		t.Errorf("a line that came back after %d others drew the pills %q, and past the look-back it is a pill of its own", recentRecordPills, said)
	}
	if frame := plainText(screen.frame()); strings.Contains(frame, string(repeatGlyph)) {
		t.Errorf("a count is drawn for a line that came back past the look-back:\n%s", frame)
	}
}

func TestARecordLineThatComesBackJustInsideTheLookBackIsCounted(t *testing.T) {
	screen := aTrialScreen()
	send(screen, aRecordLine(theTaskDoneLine))
	for step := 1; step < recentRecordPills; step++ {
		send(screen, aRecordLine("job 2 task "+strconv.Itoa(step+1)+" started · Add the seven tetrominoes"))
	}
	send(screen, aRecordLine(theTaskDoneLine))

	said := pillTexts(screen)
	if len(said) != recentRecordPills || said[0] != theTaskDoneLine {
		t.Errorf("a line that came back after %d others drew the pills %q, and inside the look-back it is counted on the pill it has", recentRecordPills-1, said)
	}
	if counts := recordPillCounts(screen); len(counts) == 0 || counts[0] != 2 {
		t.Errorf("the counts on the pills are %v, and the first pill is the one counted twice", counts)
	}
}
