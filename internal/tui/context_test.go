package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/testkit"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// aStatusWithTheContextInUse is what the running program sends after a model
// call: how many tokens that call held, how many the model can hold at once, and
// what the session has cost so far.
func aStatusWithTheContextInUse(held string, window string) contract.SocketEnvelope {
	return contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldModel:         "local",
		contract.StatusFieldContextTokens: held,
		contract.StatusFieldContextWindow: window,
		contract.StatusFieldTokensIn:      "6.1k",
		contract.StatusFieldTokensOut:     "0.4k",
		contract.StatusFieldCost:          "$0.04",
	}}
}

// attachedWith is a screen that has taken one status message from the program.
func attachedWith(envelope contract.SocketEnvelope) *Screen {
	screen, _ := screenWithLink()
	screen.Update(linkMessage{up: true})
	send(screen, envelope)
	return screen
}

// theEmptyMeter is the ten-cell context meter with one cell filled, which is
// what five percent of the window draws.
const theEmptyMeter = "▰▱▱▱▱▱▱▱▱▱"

func TestTheHeaderShowsHowMuchOfTheModelsContextIsInUse(t *testing.T) {
	screen := attachedWith(aStatusWithTheContextInUse("12400", "262144"))

	header := headerOf(screen)
	if !strings.Contains(header, "ctx "+theEmptyMeter+" 5%") {
		t.Errorf("the header is %q, and it should hold %q", header, "ctx "+theEmptyMeter+" 5%")
	}
	if !strings.Contains(header, "6.1k in 0.4k out · $0.04") {
		t.Errorf("the header is %q, and the session tokens and money stay after the context measure", header)
	}
	if strings.Index(header, "local") > strings.Index(header, "ctx ") {
		t.Errorf("the header is %q, and the context measure comes after the model alias", header)
	}
	if strings.Index(header, "ctx ") > strings.Index(header, "6.1k in") {
		t.Errorf("the header is %q, and the session cost comes after the context measure", header)
	}
}

// aStatusWithEverything is a status carrying every piece the header can draw:
// the model, the context, the cache, a running task that began twelve minutes
// before the test's clock, its round, and the session's cost.
func aStatusWithEverything() contract.SocketEnvelope {
	return contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldModel:         "opus",
		contract.StatusFieldContextTokens: "26800",
		contract.StatusFieldContextWindow: "262144",
		contract.StatusFieldCachedTokens:  "24656",
		contract.StatusFieldTask:          "17",
		contract.StatusFieldTaskState:     "running",
		contract.StatusFieldTaskStarted:   startOfTest.Add(-12 * time.Minute).Format(time.RFC3339),
		contract.StatusFieldRound:         "27",
		contract.StatusFieldTokensIn:      "6.1k",
		contract.StatusFieldTokensOut:     "0.4k",
		contract.StatusFieldCost:          "$0.04",
	}}
}

// TestTheHeaderSaysTheCacheTheElapsedTimeAndTheRound holds the three pieces
// the second look added to the header, in the order they sit: the task's
// elapsed time and round after the task, and the cache share, coloured by how
// warm it is, before the cost.
func TestTheHeaderSaysTheCacheTheElapsedTimeAndTheRound(t *testing.T) {
	screen, _ := newTestScreen(160, 24)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithEverything())

	header := plainText(headerOf(screen))
	wanted := "opus · ctx ▰▱▱▱▱▱▱▱▱▱ 10% · task 17 running · 12m · r27 · cache 92% · 6.1k in 0.4k out · $0.04"
	if !strings.Contains(header, wanted) {
		t.Errorf("the header is %q, and it should read %q", header, wanted)
	}
	for _, one := range []struct {
		cached string
		drawn  style
		saying string
	}{
		{"24656", styleDone, "a cache at ninety-two percent is warm and green"},
		{"13400", styleWarn, "a cache at fifty percent is amber"},
		{"2680", styleBad, "a cache at ten percent is cold and red"},
	} {
		send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{contract.StatusFieldCachedTokens: one.cached}})
		found := false
		for _, piece := range screen.headerParts() {
			if strings.HasPrefix(piece.text, "cache ") {
				found = true
				if piece.style != one.drawn {
					t.Errorf("the piece %q is drawn in style %d, and %s", piece.text, piece.style, one.saying)
				}
			}
		}
		if !found {
			t.Errorf("the header has no cache piece with %s cached tokens", one.cached)
		}
	}
	send(screen, contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{contract.StatusFieldCachedTokens: "0"}})
	if strings.Contains(plainText(headerOf(screen)), "cache") {
		t.Errorf("the header is %q, and a cache of nothing is not a fact worth a piece", plainText(headerOf(screen)))
	}
}

// TestTheHeaderDropsWholePiecesFromTheRightUntilItFits holds that at eighty
// columns the header keeps the wordmark, the model, the meter and the task,
// drops the least important pieces whole from the right, never cuts a piece
// in half, and still finds room for the health mark.
func TestTheHeaderDropsWholePiecesFromTheRightUntilItFits(t *testing.T) {
	screen, _ := newTestScreen(80, 24)
	screen.Update(linkMessage{up: true})
	send(screen, aStatusWithEverything())

	header := plainText(headerOf(screen))
	for _, kept := range []string{"NERDGENIE", "opus", "ctx ▰▱▱▱▱▱▱▱▱▱ 10%", "task 17 running", "● healthy"} {
		if !strings.Contains(header, kept) {
			t.Errorf("at eighty columns the header is %q, and it keeps %q", header, kept)
		}
	}
	for _, dropped := range []string{"cache", "$0.04", "6.1k"} {
		if strings.Contains(header, dropped) {
			t.Errorf("at eighty columns the header is %q, and %q is dropped before the task and the meter", header, dropped)
		}
	}
	if displayWidth(header) > 80 {
		t.Errorf("the header is %d columns wide", displayWidth(header))
	}
	pieces := strings.Split(strings.TrimSpace(strings.TrimSuffix(header, "● healthy")), " · ")
	for _, piece := range pieces {
		if strings.HasSuffix(piece, "in") || strings.HasSuffix(piece, "runnin") || strings.HasSuffix(piece, "$0") {
			t.Errorf("the header holds the piece %q cut in half, and it drops whole pieces", piece)
		}
	}
}

func TestTheHeaderSaysNothingAboutTheContextUntilTheProgramSendsBothNumbers(t *testing.T) {
	for name, fields := range map[string]map[string]string{
		"neither number": {contract.StatusFieldModel: "local"},
		"only the tokens": {
			contract.StatusFieldModel:         "local",
			contract.StatusFieldContextTokens: "12400",
		},
		"only the window": {
			contract.StatusFieldModel:         "local",
			contract.StatusFieldContextWindow: "262144",
		},
		"a window of nothing": {
			contract.StatusFieldModel:         "local",
			contract.StatusFieldContextTokens: "12400",
			contract.StatusFieldContextWindow: "0",
		},
	} {
		screen := attachedWith(contract.SocketEnvelope{Type: contract.SocketStatus, Fields: fields})
		if header := headerOf(screen); strings.Contains(header, "ctx ") {
			t.Errorf("with %s the header is %q, and it must not measure a context it was not told about", name, header)
		}
	}
}

// TestTheContextMeterTurnsGreenAmberAndRedAsItFills holds the three colours
// of the meter and the share beside it: green while there is room to spare,
// amber from half, and red from eighty percent, on the filled cells and on
// the number alike, with the empty cells always muted.
func TestTheContextMeterTurnsGreenAmberAndRedAsItFills(t *testing.T) {
	for _, one := range []struct {
		held   string
		shown  string
		drawn  style
		saying string
	}{
		{held: "12400", shown: " 5%", drawn: styleDone, saying: "a context with room to spare is green"},
		{held: "131072", shown: " 50%", drawn: styleWarn, saying: "half is amber"},
		{held: "207093", shown: " 79%", drawn: styleWarn, saying: "seventy-nine percent is still amber"},
		{held: "209716", shown: " 80%", drawn: styleBad, saying: "eighty percent is red"},
		{held: "262144", shown: " 100%", drawn: styleBad, saying: "a full context is red"},
	} {
		screen := attachedWith(aStatusWithTheContextInUse(one.held, "262144"))
		share, filled, empty := span{}, span{}, span{}
		for _, piece := range screen.headerParts() {
			switch {
			case strings.HasSuffix(piece.text, "%"):
				share = piece
			case strings.HasPrefix(piece.text, string(barFullGlyph)):
				filled = piece
			case strings.HasPrefix(piece.text, string(barEmptyGlyph)):
				empty = piece
			}
		}
		if share.text != one.shown {
			t.Errorf("%s tokens drew the share as %q, and it is %q", one.held, share.text, one.shown)
			continue
		}
		if share.style != one.drawn || filled.style != one.drawn {
			t.Errorf("the share %s is drawn in style %d and the filled cells in %d, and %s", one.shown, share.style, filled.style, one.saying)
		}
		if empty.text != "" && empty.style != styleMeterOff {
			t.Errorf("the empty cells %q are drawn in style %d, and they are always muted", empty.text, empty.style)
		}
	}
}

func TestTheHeaderIsDrawnAsTheGoldenFilesHaveItWithAndWithoutTheContextMeasure(t *testing.T) {
	with := attachedWith(aStatusWithTheContextInUse("12400", "262144"))
	testkit.Golden(t, "context-measure-80x24.txt", []byte(with.frame()))

	without := attachedWith(contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldModel:     "local",
		contract.StatusFieldTokensIn:  "6.1k",
		contract.StatusFieldTokensOut: "0.4k",
		contract.StatusFieldCost:      "$0.04",
	}})
	testkit.Golden(t, "no-context-measure-80x24.txt", []byte(without.frame()))
}
