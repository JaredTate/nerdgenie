package context

import (
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// MaxStandingOrderLines is how much of a work folder's AGENTS.md rides in
// front of the model. A standing order is the project's rules and how to run
// and test it, and it is read on every call of every task in that folder, so a
// long one would crowd the task out of the window; Home Recon's is a hundred
// and fifty lines of hard rules. Sixty lines is a page, which is what a
// colleague reads on day one.
const MaxStandingOrderLines = 60

// TheStandingOrderCutLine is the line under a standing order that was cut.
const TheStandingOrderCutLine = "... cut at sixty lines; read AGENTS.md for the rest"

// standingOrderMessage is the standing order as one message under the job
// summary, its heading naming the file and how long it is, its body cut at
// MaxStandingOrderLines with a line saying so.
func standingOrderMessage(text string) contract.Message {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	shown := lines
	if len(shown) > MaxStandingOrderLines {
		shown = append(append([]string{}, lines[:MaxStandingOrderLines]...), TheStandingOrderCutLine)
	}
	return asUserMessage(fmt.Sprintf(standingOrderHeading, len(lines)), strings.Join(shown, "\n"))
}
