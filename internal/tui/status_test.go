package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// headerOf is the first row of the frame.
func headerOf(screen *Screen) string {
	return strings.Split(screen.View(), "\n")[0]
}

// aFullStatus is what the running program says about itself in the middle of the
// example task docs/TUI_DESIGN.md draws.
func aFullStatus() contract.SocketEnvelope {
	return contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		"model":     "opus",
		"task":      "task 17",
		"taskState": "running",
		"tokensIn":  "6.1k",
		"tokensOut": "0.4k",
		"cost":      "$0.04",
		"budget":    "86 rounds, 51 min left",
		"state":     "thinking",
	}}
}

func TestAStatusMessageFillsTheHeaderAndTheStatusStrip(t *testing.T) {
	screen, _ := screenWithLink()
	screen.Update(linkMessage{up: true})
	send(screen, aFullStatus())

	header := headerOf(screen)
	for _, wanted := range []string{"coeus", "opus", "task 17 running", "6.1k in 0.4k out", "$0.04"} {
		if !strings.Contains(header, wanted) {
			t.Errorf("the header is %q and it should hold %q", header, wanted)
		}
	}
	strip := statusStrip(screen)
	for _, wanted := range []string{"thinking", "86 rounds, 51 min left"} {
		if !strings.Contains(strip, wanted) {
			t.Errorf("the status strip is %q and it should hold %q", strip, wanted)
		}
	}
}

func TestTheStatusStripNamesTheToolThatIsRunning(t *testing.T) {
	screen, _ := screenWithLink()
	screen.Update(linkMessage{up: true})
	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		"state": "using",
		"tool":  "read",
	}})

	if !strings.Contains(statusStrip(screen), "using read") {
		t.Errorf("the status strip is %q, and it names the tool that is running", statusStrip(screen))
	}
}

func TestAFinishedToolCallIsOneDimLineInTheTranscript(t *testing.T) {
	screen, _ := screenWithLink()
	wanted := "read memory/product.md · 2,100 characters · r3"
	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{"toolLine": wanted}})

	frame := screen.View()
	if !strings.Contains(frame, string(toolArrowGlyph)+" "+wanted) {
		t.Errorf("the frame does not hold the tool line %q under the arrow", wanted)
	}
}

func TestTheHealthDotGoesQuietWhenTheProgramStopsAnswering(t *testing.T) {
	screen, clock := newTestScreen(80, 24)
	screen.link = &recordingLink{}
	screen.Update(linkMessage{up: true})
	if !strings.Contains(headerOf(screen), "healthy") {
		t.Fatal("the health dot is not filled just after the screen attached")
	}

	advance(screen, clock, healthFreshFor+time.Second)
	if strings.Contains(headerOf(screen), "healthy") {
		t.Errorf("the header is %q, and the program has not answered for longer than the health check allows", headerOf(screen))
	}
	if !strings.Contains(headerOf(screen), "quiet") {
		t.Errorf("the header is %q, and a link with no recent answer shows a hollow dot", headerOf(screen))
	}

	send(screen, aFullStatus())
	if !strings.Contains(headerOf(screen), "healthy") {
		t.Error("the health dot did not go back to filled when the program answered again")
	}
}

func TestAStatusFieldTheScreenDoesNotKnowIsIgnored(t *testing.T) {
	screen, _ := screenWithLink()
	screen.Update(linkMessage{up: true})
	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		"model":              "opus",
		"somethingBrandNew":  "from a newer program",
		"anotherUnknownName": "also new",
	}})

	if !strings.Contains(headerOf(screen), "opus") {
		t.Error("a field the screen does not know made it drop the ones it does know")
	}
}

func TestAnUnknownStateWordLeavesTheScreenAsItWas(t *testing.T) {
	screen, _ := screenWithLink()
	screen.Update(linkMessage{up: true})
	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{"state": "thinking"}})
	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{"state": "somethingelse"}})

	if !strings.Contains(statusStrip(screen), "thinking") {
		t.Errorf("the status strip is %q, and a word the screen does not know must not make it lie", statusStrip(screen))
	}
}
