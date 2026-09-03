package tui

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// aStatusWithAPlan is what the program says about itself while a task is
// running: everything the header already reads, and the plan and the job count
// the side panel reads beside it.
func aStatusWithAPlan() contract.SocketEnvelope {
	return contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldModel:         "opus",
		contract.StatusFieldTask:          "17",
		contract.StatusFieldTaskState:     "running",
		contract.StatusFieldTokensIn:      "6.1k",
		contract.StatusFieldTokensOut:     "0.4k",
		contract.StatusFieldCost:          "$0.04",
		contract.StatusFieldBudget:        "86 rounds, 51 min left",
		contract.StatusFieldContextTokens: "12400",
		contract.StatusFieldContextWindow: "262144",
		contract.StatusFieldState:         contract.StateThinking,
		statusFieldPlan: "[x] the product notes are read\n" +
			"[x] a draft under 280 characters is written\n" +
			"[ ] the tweet is posted",
		statusFieldJobs: "3",
	}}
}

// panelColumnOf is the rows of the side panel alone, cut out of the frame at the
// column the panel begins in. A row of the frame that is not the panel's, such
// as the header or a rule, is left out, because the panel's rows are the ones
// carrying its edge.
func panelColumnOf(screen *Screen) []string {
	drawn := []string{}
	for _, line := range strings.Split(plainText(screen.frame()), "\n") {
		letters := []rune(line)
		if len(letters) < screen.transcriptColumns() {
			continue
		}
		beside := strings.TrimRight(string(letters[screen.transcriptColumns():]), " ")
		if !strings.HasPrefix(beside, string(panelEdgeGlyph)) {
			continue
		}
		drawn = append(drawn, beside)
	}
	return drawn
}

func TestTheSidePanelIsDrawnFromAHundredColumnsAndNotBelowThem(t *testing.T) {
	for _, size := range []struct {
		width  int
		panel  bool
		saying string
	}{{99, false, "ninety-nine columns"}, {100, true, "a hundred columns"}, {120, true, "a hundred and twenty columns"}} {
		screen, _ := newTestScreen(size.width, 36)
		screen.Update(linkMessage{up: true})
		send(screen, aStatusWithAPlan())

		frame := plainText(screen.frame())
		if shown := strings.Contains(frame, "the tweet is posted"); shown != size.panel {
			t.Errorf("at %s the panel is drawn: %v, and it should be: %v\n%s", size.saying, shown, size.panel, frame)
		}
	}
}

func TestThePanelIsTwentyEightColumnsWideAndLeavesTheRestToTheTranscript(t *testing.T) {
	screen, _ := newTestScreen(120, 36)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithAPlan())
	typeAndSend(screen, theHaikuMessage)

	if columns := screen.width - screen.transcriptColumns(); columns != panelColumns {
		t.Errorf("the panel takes %d columns, and the design gives it %d", columns, panelColumns)
	}
	for _, line := range strings.Split(plainText(screen.frame()), "\n") {
		if !strings.Contains(line, string(personBarGlyph)) {
			continue
		}
		beside := string([]rune(line)[:screen.transcriptColumns()])
		if edge := displayWidth(strings.TrimRight(beside, " ")); edge >= screen.transcriptColumns() {
			t.Errorf("the person's bubble reaches column %d, and the transcript ends at %d: %q",
				edge, screen.transcriptColumns(), line)
		}
	}
}

func TestThePanelSaysTheModelTheContextTheCostTheTaskAndTheJobs(t *testing.T) {
	screen, _ := newTestScreen(120, 36)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithAPlan())

	panel := strings.Join(panelColumnOf(screen), "\n")
	for _, wanted := range []string{"opus", "ctx 12.4k / 262k", "5%", "6.1k in 0.4k out", "$0.04", "task 17 running", "3 jobs"} {
		if !strings.Contains(panel, wanted) {
			t.Errorf("the panel does not say %q:\n%s", wanted, panel)
		}
	}
}

func TestThePanelPutsACheckBesideEveryStepThatIsDone(t *testing.T) {
	screen, _ := newTestScreen(120, 36)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithAPlan())

	panel := strings.Join(panelColumnOf(screen), "\n")
	for _, wanted := range []string{
		string(doneStepGlyph) + " the product notes are",
		string(doneStepGlyph) + " a draft under 280",
		string(toDoStepGlyph) + " the tweet is posted",
	} {
		if !strings.Contains(panel, wanted) {
			t.Errorf("the panel does not draw %q:\n%s", wanted, panel)
		}
	}
	if strings.Count(panel, string(doneStepGlyph)) != 2 {
		t.Errorf("the panel draws %d checks for the two steps that are done:\n%s", strings.Count(panel, string(doneStepGlyph)), panel)
	}
}

func TestThePanelSaysNothingTheProgramHasNotSaid(t *testing.T) {
	screen, _ := newTestScreen(120, 36)
	screen.Update(linkMessage{up: true})
	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldModel: "local",
		contract.StatusFieldState: contract.StateIdle,
	}})

	panel := strings.Join(panelColumnOf(screen), "\n")
	if !strings.Contains(panel, "local") {
		t.Errorf("the panel does not name the model, which is the one thing the program said:\n%s", panel)
	}
	for _, unwanted := range []string{"task", "job", "ctx"} {
		if strings.Contains(panel, unwanted) {
			t.Errorf("the panel says %q about a program that never said it:\n%s", unwanted, panel)
		}
	}
}

func TestThePanelIsDrawnAsTheGoldenFilesHaveIt(t *testing.T) {
	quiet, _ := newTestScreen(120, 36)
	quiet.link = &recordingLink{}
	quiet.Update(linkMessage{up: true})
	send(quiet, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldModel: "opus",
		contract.StatusFieldState: contract.StateIdle,
	}})
	testkit.Golden(t, "panel-idle-120x36.txt", []byte(quiet.frame()))

	working, _ := newTestScreen(120, 36)
	working.link = &recordingLink{}
	working.Update(linkMessage{up: true})
	send(working, aStatusWithAPlan())
	typeAndSend(working, "Post a tweet about the DigiByte anniversary. Use the product notes and keep it under 280 characters.")
	send(working, contract.SocketEnvelope{Type: contract.SocketReply, Text: "Where I stand: the notes are read, drafting next."})
	send(working, aToolLine("▸ read memory/product.md · 2,100 characters · r3"))
	testkit.Golden(t, "panel-task-120x36.txt", []byte(working.frame()))
}
