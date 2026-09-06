package browserread

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The bounds on one page as the model reads it.
const (
	// MaxElements is how many elements of a page are listed.
	MaxElements = 200
	// MaxNameRunes is how much of one element's name is shown.
	MaxNameRunes = 120
	// MaxAddressRunes is how much of a web address is shown. An address is
	// longer than a name and worth showing whole, because the model acts on it.
	MaxAddressRunes = 500
	// MaxAnswerRunes is how much of the page's answer to a question is shown.
	MaxAnswerRunes = 2000
	// MaxTextLineRunes is how much of one line of the page's text is shown. It
	// is the cap the worker keeps on the whole text, so that no one line can be
	// longer than the whole text may be.
	MaxTextLineRunes = 8000
	// cutLineRoom is the room kept back for the line that says how much of the
	// text was cut, so that the line itself never pushes a result past the cap.
	cutLineRoom = 64
)

// pageBudget is the most bytes one page may read as, which is the cap on a tool
// result before the rest spills to a file. Everything else on the page is
// written first and the text takes what is left, so that the text is cut before
// an element the model acts on ever is.
var pageBudget = contract.DefaultConfig().Caps.ToolOutputBytes

// PageText is one page as the model reads it: the address and the title, then
// one line per element, then what is below the fold and anything on the page
// that stops the agent, and last what the page says, quoted line by line. Every
// word of it that came from the page goes through fromThePage first, so that a
// page cannot write a line of its own.
func PageText(page contract.Snapshot) string {
	return pageText(page, pageBudget)
}

// ChangeText is what one action did to the page, as the model reads it: whether
// what it expected happened, what changed, and the page it left behind.
func ChangeText(change contract.Diff) string {
	written := &strings.Builder{}
	if change.ExpectationMet {
		written.WriteString("what was expected happened\n")
	} else {
		written.WriteString("not what was expected\n")
		if change.Seen != "" {
			fmt.Fprintf(written, "what happened instead: %s\n", fromThePage(change.Seen, MaxNameRunes))
		}
	}
	if !change.Settled {
		written.WriteString("the page did not come to rest before the limit, so read it again to see what it is doing\n")
	}
	if change.URLChanged {
		fmt.Fprintf(written, "the page moved to %s\n", fromThePage(change.URL, MaxAddressRunes))
	}
	if change.NewTab != "" {
		fmt.Fprintf(written, "a new tab opened, %s\n", fromThePage(change.NewTab, MaxNameRunes))
	}
	if len(change.NewElements) > 0 {
		written.WriteString("new on the page:\n" + elementsText(change.NewElements))
	}
	written.WriteString(dialogText(change.Dialog))
	written.WriteString(downloadText(change.Download))
	written.WriteString(wallText(change.Wall))
	before := written.String() + "---\n"
	return before + pageText(change.Snapshot, pageBudget-len(before))
}

// pageText is one page inside the room it is given: everything but the text
// first, then the text cut to what is left.
func pageText(page contract.Snapshot, room int) string {
	written := &strings.Builder{}
	fmt.Fprintf(written, "%s\n%s\n", fromThePage(page.Title, MaxNameRunes), fromThePage(page.URL, MaxAddressRunes))
	if page.TabID != "" {
		fmt.Fprintf(written, "tab %s\n", fromThePage(page.TabID, MaxNameRunes))
	}
	written.WriteString(elementsText(page.Elements))
	if page.HiddenYetDrawn > 0 {
		written.WriteString(theHiddenYetDrawnLine(page.HiddenYetDrawn))
	}
	if page.BelowFold > 0 {
		fmt.Fprintf(written, "%d more elements below the fold\n", page.BelowFold)
	}
	written.WriteString(dialogText(page.Dialog))
	written.WriteString(downloadText(page.Download))
	written.WriteString(wallText(page.Wall))
	written.WriteString(answerText(page.Answer))
	written.WriteString(errorsText(page.Errors))
	written.WriteString(textSection(page.Text, room-written.Len()))
	return written.String()
}

// answerText is the page's answer to what the read asked, after the outline
// and before the errors, and nothing when nothing was asked.
func answerText(answer string) string {
	if answer == "" {
		return ""
	}
	return "the page answered: " + fromThePage(answer, MaxAnswerRunes) + "\n"
}

// errorsText is what went wrong on the page, one line each, after the outline
// and before the text, so that it is never cut and the model reads it before
// it reads what the page says. On the live game build the page's script had
// answered 404, the game never started, and nothing in the result said so.
func errorsText(errors []string) string {
	if len(errors) == 0 {
		return ""
	}
	written := &strings.Builder{}
	written.WriteString("page errors:\n")
	for _, line := range errors {
		fmt.Fprintf(written, "- %s\n", fromThePage(line, MaxNameRunes))
	}
	return written.String()
}

// TheHiddenYetDrawnMark follows an element whose markup says hidden and which
// the browser draws all the same. It says the cause, because the model that
// built the fresh Tetris game read every overlay's heading at once, and saw
// the GAME OVER card on top of the start card in its own screenshot, without
// working out why: the page's own display rule was beating the attribute.
const TheHiddenYetDrawnMark = " (marked hidden, yet drawn: a style rule overrides the hidden attribute)"

// theHiddenYetDrawnLine is the page's own count of nodes its markup hides and
// a style rule draws, with the one rule that fixes every one of them. It rides
// above the elements because a badge with no role never reaches the outline,
// and because the model that built the fresh game fixed the one overlay it was
// told about and left the badge and the touch controls drawn.
func theHiddenYetDrawnLine(count int) string {
	return fmt.Sprintf("%d elements are hidden in the markup yet drawn: a style rule overrides the hidden attribute, and one rule fixes every one: [hidden] { display: none !important; }\n", count)
}

// elementsText is one line per element, in reading order, up to the cap.
func elementsText(elements []contract.Element) string {
	written := &strings.Builder{}
	for at, element := range elements {
		if at >= MaxElements {
			fmt.Fprintf(written, "... %d more elements, so narrow the page before acting\n", len(elements)-MaxElements)
			break
		}
		marks := ""
		if element.New {
			marks += " (new)"
		}
		if element.HiddenYetDrawn {
			marks += TheHiddenYetDrawnMark
		}
		fmt.Fprintf(written, "%s %s %q%s\n",
			fromThePage(element.Ref, MaxNameRunes), fromThePage(element.Role, MaxNameRunes),
			fromThePage(element.Name, MaxNameRunes), marks)
	}
	return written.String()
}

// textSection is what the page says, after its elements: every line quoted
// behind "> " so that none of them can read as a line of the tool's own, and
// cut to the room left under the cap on a tool result, with a last line saying
// how much was cut. Once one line does not fit, every line after it is cut too,
// so that what the model sees is always the top of the page in reading order.
func textSection(text string, room int) string {
	lines := textLines(text)
	if len(lines) == 0 {
		return ""
	}
	written := &strings.Builder{}
	written.WriteString("text on the page:\n")
	room -= written.Len() + cutLineRoom
	cut := 0
	for _, line := range lines {
		quoted := "> " + line + "\n"
		if cut == 0 && len(quoted) <= room {
			written.WriteString(quoted)
			room -= len(quoted)
			continue
		}
		cut += utf8.RuneCountInString(line) + 1
	}
	if cut > 0 {
		fmt.Fprintf(written, "... %d more characters of the page's text were cut\n", cut)
	}
	return written.String()
}

// textLines is the page's text as lines, each through the same door as every
// other word from the page, with the blank ones left out.
func textLines(text string) []string {
	lines := []string{}
	for _, line := range strings.Split(text, "\n") {
		if clean := fromThePage(line, MaxTextLineRunes); clean != "" {
			lines = append(lines, clean)
		}
	}
	return lines
}

// dialogText is the line a dialog box gets, which is none when there is none.
func dialogText(dialog *contract.Dialog) string {
	if dialog == nil {
		return ""
	}
	return fmt.Sprintf("a %s dialog is open, saying %q; the tab is blocked until it is answered\n",
		fromThePage(dialog.Kind, MaxNameRunes), fromThePage(dialog.Message, MaxNameRunes))
}

// downloadText is the line a download gets, which is none when there is none.
func downloadText(download *contract.Download) string {
	if download == nil {
		return ""
	}
	return fmt.Sprintf("the page downloaded %s, saved at %s\n",
		fromThePage(download.Filename, MaxNameRunes), fromThePage(download.Path, MaxAddressRunes))
}

// wallText is the line one of the three walls gets, which is none when the page
// shows none.
func wallText(wall *contract.Wall) string {
	if wall == nil {
		return ""
	}
	return fmt.Sprintf("this page shows a %s wall (%s), so hand the browser to the user rather than working round it\n",
		fromThePage(string(wall.Kind), MaxNameRunes), fromThePage(wall.Detail, MaxNameRunes))
}

// fromThePage is how every word that came from the page is written into a
// result: on one line, inside a cap, and with the three angle brackets that open
// and close the marker the working context wraps a tool result in taken out. A
// page can call an element anything at all, and a page that writes a newline
// into its title can otherwise draw a line that reads as the tool's own.
func fromThePage(text string, length int) string {
	flattened := strings.Join(strings.Fields(text), " ")
	flattened = strings.ReplaceAll(flattened, "<<<", " ")
	flattened = strings.ReplaceAll(flattened, ">>>", " ")
	letters := []rune(strings.TrimSpace(flattened))
	if len(letters) <= length {
		return string(letters)
	}
	return string(letters[:length]) + "..."
}
