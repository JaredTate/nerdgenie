package loop

import (
	"context"
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// A page whose script never yields. The browser tool says so, and says to
// look for a loop whose condition never changes; the fifth game build's
// play-test task heard that and wrote nine play-test drivers instead of
// reading the three while loops in the game's script, which an earlier task
// of the job had written. The harness can do the looking: when a browser
// result reports a hung page, it reads the page and the scripts it loads from
// the local server, or the disk, and lists their loops by file and line on
// the same result, so the model reads three lines rather than guessing for
// two hours.

// TheLoopsLine opens the list of loops on a hung page's result.
const TheLoopsLine = "the loops in the scripts this page runs, one of which may be the one that never yields:"

// TheHungPageFact opens the situation line that keeps a hung page's marked
// loop in front of the model every round, because a list on one result is
// read once and the live model went back to the sound engine after reading it.
const TheHungPageFact = "browser: the page hangs on the last action in a loop that looks endless, so fix this loop before anything else: "

// theSignsOfAHungPage are the words a browser result carries when the page's
// own script kept it busy.
var theSignsOfAHungPage = []string{"keeping it busy", "does not yield"}

// MaxLoopsListed is the most loop lines the list holds.
const MaxLoopsListed = 12

// listTheLoopsAfter reads the page's scripts when a browser result reports a
// hung page, and hands back the list of their loops to put on the result, or
// nothing when the result is ordinary, no page is known, or the page is not
// on this machine.
func (running *run) listTheLoopsAfter(ctx context.Context, call contract.ToolCall, text string) string {
	if !strings.HasPrefix(call.Name, "browser") || !saysThePageHung(text) || !isAPageOnThisMachine(running.pageAddress) {
		return ""
	}
	whiles, fors := []string{}, []string{}
	for _, script := range theScriptsOf(ctx, running.pageAddress) {
		itsWhiles, itsFors := loopLinesIn(script)
		whiles, fors = append(whiles, itsWhiles...), append(fors, itsFors...)
	}
	lines := append(whiles, fors...)
	if len(lines) == 0 {
		return ""
	}
	if len(lines) > MaxLoopsListed {
		left := len(lines) - MaxLoopsListed
		lines = append(lines[:MaxLoopsListed], fmt.Sprintf("and %d more for loops", left))
	}
	return TheLoopsLine + "\n" + strings.Join(lines, "\n")
}

// saysThePageHung says whether a browser result reports a page whose own
// script kept it busy.
func saysThePageHung(text string) bool {
	for _, sign := range theSignsOfAHungPage {
		if strings.Contains(text, sign) {
			return true
		}
	}
	return false
}

// TheMarkArrow opens the mark on a loop line, and is what a marked line is
// known by, so that a result written under an older wording of the mark still
// reads as marked when a pick-up replays the log.
const TheMarkArrow = " <- "

// theMarkedLoopIn is the first loop line on a result that carries the mark,
// without the mark, or empty when none does.
func theMarkedLoopIn(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if loopLine, _, marked := strings.Cut(line, TheMarkArrow); marked && strings.Contains(loopLine, ":") && (strings.Contains(loopLine, "while") || strings.Contains(loopLine, "for")) {
			return loopLine
		}
	}
	return ""
}

// pageTitleIn is the page's title as a browser result carries it, the line
// before its address, or the result's first line when it carries no address:
// a click's first line is the judge's verdict, and the screen read "page: not
// what was expected" for a whole sitting.
func pageTitleIn(text string) string {
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		address := addressIn(line)
		if index == 0 || address == "" {
			continue
		}
		if title := strings.TrimSpace(lines[index-1]); title != "" && title != "---" {
			return title
		}
		return address
	}
	return firstLine(text)
}

// addressIn is the page's address as a browser result carries it on one of
// its first lines, or empty when the result carries none.
func addressIn(text string) string {
	lines := strings.SplitN(text, "\n", 5)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") || strings.HasPrefix(line, "file:") {
			return line
		}
	}
	return ""
}
