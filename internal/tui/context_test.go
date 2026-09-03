package tui

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/testkit"

	"github.com/JaredTate/coeus/internal/contract"
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

func TestTheHeaderShowsHowMuchOfTheModelsContextIsInUse(t *testing.T) {
	screen := attachedWith(aStatusWithTheContextInUse("12400", "262144"))

	header := headerOf(screen)
	if !strings.Contains(header, "ctx 12.4k / 262k · 5%") {
		t.Errorf("the header is %q, and it should hold %q", header, "ctx 12.4k / 262k · 5%")
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

func TestTheContextShareIsWarningColouredAtEightyAndErrorColouredAtNinetyFive(t *testing.T) {
	for _, one := range []struct {
		held   string
		shown  string
		drawn  style
		saying string
	}{
		{held: "12400", shown: "5%", drawn: styleDim, saying: "a context with room to spare is quiet"},
		{held: "209716", shown: "80%", drawn: styleWarn, saying: "eighty percent is the warning"},
		{held: "246136", shown: "94%", drawn: styleWarn, saying: "ninety-four percent is still the warning"},
		{held: "249037", shown: "95%", drawn: styleError, saying: "ninety-five percent is the error colour"},
		{held: "262144", shown: "100%", drawn: styleError, saying: "a full context is the error colour"},
	} {
		screen := attachedWith(aStatusWithTheContextInUse(one.held, "262144"))
		share := span{}
		for _, piece := range screen.headerParts() {
			if strings.HasSuffix(piece.text, "%") {
				share = piece
			}
		}
		if share.text != one.shown {
			t.Errorf("%s tokens drew the share as %q, and it is %q", one.held, share.text, one.shown)
			continue
		}
		if share.style != one.drawn {
			t.Errorf("the share %s is drawn in style %d, and %s", one.shown, share.style, one.saying)
		}
	}
}

func TestTheHeaderIsDrawnAsTheGoldenFilesHaveItWithAndWithoutTheContextMeasure(t *testing.T) {
	with := attachedWith(aStatusWithTheContextInUse("12400", "262144"))
	testkit.Golden(t, "context-measure-80x24.txt", []byte(with.View()))

	without := attachedWith(contract.SocketEnvelope{Type: contract.SocketStatus, Fields: map[string]string{
		contract.StatusFieldModel:     "local",
		contract.StatusFieldTokensIn:  "6.1k",
		contract.StatusFieldTokensOut: "0.4k",
		contract.StatusFieldCost:      "$0.04",
	}})
	testkit.Golden(t, "no-context-measure-80x24.txt", []byte(without.View()))
}
