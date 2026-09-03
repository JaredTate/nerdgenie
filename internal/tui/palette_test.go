package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/JaredTate/coeus/internal/contract"
)

// theCommands are the slash commands the palette tests offer.
func theCommands() []contract.Command {
	return []contract.Command{
		{Name: "help", Help: "Shows every command."},
		{Name: "status", Help: "Shows the model, the cost, the jobs, and the health."},
		{Name: "stop", Help: "Stops this turn."},
	}
}

// screenWithCommands builds a screen that already knows some slash commands.
func screenWithCommands() (*Screen, *recordingLink) {
	clock := testkitClock()
	screen := New(Options{
		Clock:       clock,
		Environment: plainEnvironment,
		Commands:    theCommands(),
		Width:       80,
		Height:      24,
	})
	link := &recordingLink{}
	screen.link = link
	return screen, link
}

func TestTypingASlashOpensThePaletteAndListsEveryCommand(t *testing.T) {
	screen, _ := screenWithCommands()
	press(screen, '/')

	frame := screen.frame()
	for _, one := range theCommands() {
		if !strings.Contains(frame, "/"+one.Name) {
			t.Errorf("the palette does not list /%s", one.Name)
		}
		if !strings.Contains(frame, one.Help) {
			t.Errorf("the palette does not show the help line for /%s", one.Name)
		}
	}
}

func TestThePaletteFiltersAsThePersonTypes(t *testing.T) {
	screen, _ := screenWithCommands()
	typeWord(screen, "/st")

	frame := screen.frame()
	if !strings.Contains(frame, "/status") || !strings.Contains(frame, "/stop") {
		t.Error("the palette dropped a command that still matches what was typed")
	}
	if strings.Contains(frame, "/help") {
		t.Error("the palette still lists /help, which does not match what was typed")
	}
}

func TestThePaletteClosesWhenNothingMatchesAndWhenTheSlashGoes(t *testing.T) {
	screen, _ := screenWithCommands()
	typeWord(screen, "/zzz")
	if strings.Contains(screen.frame(), "Shows every command.") {
		t.Error("the palette is still listing commands when nothing matches what was typed")
	}

	screen, _ = screenWithCommands()
	typeWord(screen, "/h")
	pressKey(screen, tea.KeyBackspace)
	pressKey(screen, tea.KeyBackspace)
	if screen.paletteOpen {
		t.Error("the palette is still open after the slash was deleted")
	}
}

func TestTabCompletesTheFirstMatchAndClosesThePalette(t *testing.T) {
	screen, _ := screenWithCommands()
	typeWord(screen, "/sta")
	pressKey(screen, tea.KeyTab)

	if screen.input.text() != "/status " {
		t.Errorf("the input box holds %q, and Tab completes the command that matches", screen.input.text())
	}
	if screen.paletteOpen {
		t.Error("the palette is still open after the command was completed")
	}
}

func TestEnterCompletesFromThePaletteAndTheNextEnterSendsIt(t *testing.T) {
	screen, link := screenWithCommands()
	typeWord(screen, "/hel")
	pressKey(screen, tea.KeyEnter)

	if len(link.sent) != 0 {
		t.Fatalf("the first Enter sent %+v, and while the palette is open Enter completes", link.sent)
	}
	if screen.input.text() != "/help " {
		t.Fatalf("the input box holds %q after Enter completed the command", screen.input.text())
	}

	pressKey(screen, tea.KeyEnter)
	if len(link.sent) != 1 || link.sent[0].Type != contract.SocketCommand || link.sent[0].Text != "help" {
		t.Errorf("the second Enter sent %+v, and it runs the command in the box", link.sent)
	}
}

func TestEscapeClosesThePaletteAndLeavesWhatWasTyped(t *testing.T) {
	screen, _ := screenWithCommands()
	typeWord(screen, "/st")
	pressKey(screen, tea.KeyEsc)

	if screen.paletteOpen {
		t.Error("Escape did not close the palette")
	}
	if screen.input.text() != "/st" {
		t.Errorf("Escape changed what was typed to %q", screen.input.text())
	}
}

func TestThePaletteLearnsTheCommandsTheProgramReports(t *testing.T) {
	screen, _ := screenWithLink()
	send(screen, contract.SocketEnvelope{
		Type:   contract.SocketStatus,
		Fields: map[string]string{contract.StatusFieldCommands: "jobs" + contract.StatusCommandSeparator + "Lists every job.\ncron" + contract.StatusCommandSeparator + "Lists the jobs that have a schedule."},
	})
	press(screen, '/')

	frame := screen.frame()
	if !strings.Contains(frame, "/jobs") || !strings.Contains(frame, "Lists every job.") {
		t.Error("the palette did not learn the commands the program reported")
	}
	if !strings.Contains(frame, "/cron") {
		t.Error("the palette learned only the first command the program reported")
	}
}
