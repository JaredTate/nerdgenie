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
)

// PageText is one page as the model reads it: the address and the title, then
// one line per element, then what is below the fold and anything on the page
// that stops the agent.
func PageText(page contract.Snapshot) string {
	written := &strings.Builder{}
	fmt.Fprintf(written, "%s\n%s\n", page.Title, page.URL)
	if page.TabID != "" {
		fmt.Fprintf(written, "tab %s\n", page.TabID)
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
			fmt.Fprintf(written, "what happened instead: %s\n", change.Seen)
		}
	}
	if !change.Settled {
		written.WriteString("the page did not come to rest before the limit, so read it again to see what it is doing\n")
	}
	if change.URLChanged {
		fmt.Fprintf(written, "the page moved to %s\n", change.URL)
	}
	if change.NewTab != "" {
		fmt.Fprintf(written, "a new tab opened, %s\n", change.NewTab)
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
		fmt.Fprintf(written, "%s %s %q%s\n", element.Ref, element.Role, cutRunes(element.Name, MaxNameRunes), isNew)
	}
	return written.String()
}

// dialogText is the line a dialog box gets, which is none when there is none.
func dialogText(dialog *contract.Dialog) string {
	if dialog == nil {
		return ""
	}
	return fmt.Sprintf("a %s dialog is open, saying %q; the tab is blocked until it is answered\n",
		dialog.Kind, cutRunes(dialog.Message, MaxNameRunes))
}

// downloadText is the line a download gets, which is none when there is none.
func downloadText(download *contract.Download) string {
	if download == nil {
		return ""
	}
	return fmt.Sprintf("the page downloaded %s, saved at %s\n", download.Filename, download.Path)
}

// wallText is the line one of the three walls gets, which is none when the page
// shows none.
func wallText(wall *contract.Wall) string {
	if wall == nil {
		return ""
	}
	return fmt.Sprintf("this page shows a %s wall (%s), so hand the browser to the user rather than working round it\n",
		wall.Kind, wall.Detail)
}

// cutRunes cuts a name to a number of characters, because a page can call an
// element anything at all.
func cutRunes(name string, length int) string {
	flattened := strings.Join(strings.Fields(name), " ")
	letters := []rune(flattened)
	if len(letters) <= length {
		return flattened
	}
	return string(letters[:length]) + "..."
}
