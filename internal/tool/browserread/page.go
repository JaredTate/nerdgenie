package browserread

import (
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
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
)

// PageText is one page as the model reads it: the address and the title, then
// one line per element, then what is below the fold and anything on the page
// that stops the agent. Every word of it that came from the page goes through
// fromThePage first, so that a page cannot write a line of its own.
func PageText(page contract.Snapshot) string {
	written := &strings.Builder{}
	fmt.Fprintf(written, "%s\n%s\n", fromThePage(page.Title, MaxNameRunes), fromThePage(page.URL, MaxAddressRunes))
	if page.TabID != "" {
		fmt.Fprintf(written, "tab %s\n", fromThePage(page.TabID, MaxNameRunes))
	}
	written.WriteString(elementsText(page.Elements))
	if page.BelowFold > 0 {
		fmt.Fprintf(written, "%d more elements below the fold\n", page.BelowFold)
	}
	written.WriteString(dialogText(page.Dialog))
	written.WriteString(downloadText(page.Download))
	written.WriteString(wallText(page.Wall))
	return written.String()
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
	return written.String() + "---\n" + PageText(change.Snapshot)
}

// elementsText is one line per element, in reading order, up to the cap.
func elementsText(elements []contract.Element) string {
	written := &strings.Builder{}
	for at, element := range elements {
		if at >= MaxElements {
			fmt.Fprintf(written, "... %d more elements, so narrow the page before acting\n", len(elements)-MaxElements)
			break
		}
		isNew := ""
		if element.New {
			isNew = " (new)"
		}
		fmt.Fprintf(written, "%s %s %q%s\n",
			fromThePage(element.Ref, MaxNameRunes), fromThePage(element.Role, MaxNameRunes),
			fromThePage(element.Name, MaxNameRunes), isNew)
	}
	return written.String()
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
