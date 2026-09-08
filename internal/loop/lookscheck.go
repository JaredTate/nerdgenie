package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// A "looks" check is the harness looking at a page the way a person does at
// the end of a build: at each width the line names, is anything wider than
// the window, and is the console clean. Run 23's visual QA task spent forty
// rounds proving that by hand at three widths, one screenshot and one read
// at a time. The check goes through the browser tools the model has, so the
// permission function and the log see it as they see the model's own calls,
// and its result holds the pictures it took, so the model can look at the
// width that failed and nothing else.

// TheLooksHeight is the page height every width is looked at, tall enough
// for a page laid out for a desktop or a phone and short enough to fit a
// screen.
const TheLooksHeight = 900

// TheOverflowAsk is the expression the page answers with how many pixels
// wider than the window it is, which is zero for a page that fits.
const TheOverflowAsk = "String(Math.max(0, document.documentElement.scrollWidth - window.innerWidth))"

// checkThePageLooks opens the page, then for each width resizes, photographs
// and asks the page whether it overflows and what its errors are, and stops
// at the first width that fails.
func (running *run) checkThePageLooks(ctx context.Context, url string, widths []int) (checkOutcome, error) {
	tools, missing := running.theLooksTools()
	if missing != "" {
		return checkOutcome{said: "there is no browser tool on this machine to " + missing + " with"}, nil
	}
	opened, err := running.useTheTool(ctx, tools[contract.ToolBrowserOpen], "looks-open", map[string]any{"url": url, "intent": "the done check looks at the page"})
	if err != nil {
		return checkOutcome{said: "the page could not be opened: " + err.Error()}, nil
	}
	var output strings.Builder
	output.WriteString(opened + "\n")
	var fits []string
	for _, width := range widths {
		seen, said, err := running.lookAtOneWidth(ctx, tools, width)
		output.WriteString(seen)
		if err != nil {
			return checkOutcome{said: fmt.Sprintf("at %d the page could not be looked at: %s", width, err.Error()), output: output.String()}, nil
		}
		if said != "" {
			return checkOutcome{said: said, output: output.String()}, nil
		}
		fits = append(fits, strconv.Itoa(width))
	}
	return checkOutcome{passed: true, said: "the page fits at " + joinWidths(fits) + " with no page errors", output: output.String()}, nil
}

// theLooksTools finds the four browser tools the check uses, or names the
// step the missing one was for.
func (running *run) theLooksTools() (map[string]contract.Tool, string) {
	tools := map[string]contract.Tool{}
	for _, step := range []struct{ name, does string }{
		{contract.ToolBrowserOpen, "open it"}, {contract.ToolBrowserResize, "resize it"},
		{contract.ToolBrowserScreenshot, "photograph it"}, {contract.ToolBrowserRead, "read it"},
	} {
		tool, found := running.tools().Lookup(step.name)
		if !found {
			return nil, step.does
		}
		tools[step.name] = tool
	}
	return tools, ""
}

// lookAtOneWidth resizes the page, photographs it and reads its overflow and
// its errors. It hands back what it saw, for the check's output, and the
// reason the width failed, or nothing when the page fits and is clean.
func (running *run) lookAtOneWidth(ctx context.Context, tools map[string]contract.Tool, width int) (string, string, error) {
	intent := fmt.Sprintf("the done check looks at the page %d wide", width)
	if _, err := running.useTheTool(ctx, tools[contract.ToolBrowserResize], "looks-resize", map[string]any{"intent": intent, "width": width, "height": TheLooksHeight}); err != nil {
		return "", "", err
	}
	picture, err := running.useTheTool(ctx, tools[contract.ToolBrowserScreenshot], "looks-shot", map[string]any{"intent": intent})
	if err != nil {
		return "", "", err
	}
	read, err := running.useTheTool(ctx, tools[contract.ToolBrowserRead], "looks-read", map[string]any{"intent": intent, "ask": TheOverflowAsk})
	if err != nil {
		return "", "", err
	}
	seen := fmt.Sprintf("at %d: %s\n", width, firstLine(picture))
	overflow, measured := theAnswerIn(read)
	firstError := theFirstPageErrorIn(read)
	switch {
	case !measured:
		return seen, fmt.Sprintf("at %d the page's width could not be measured", width), nil
	case overflow > 0:
		return seen, fmt.Sprintf("at %d the page overflows by %d pixels", width, overflow), nil
	case firstError != "":
		return seen, fmt.Sprintf("at %d the console holds: %s", width, firstError), nil
	}
	return seen, "", nil
}

// useTheTool runs one browser tool for the check under the time limit and
// hands back its text.
func (running *run) useTheTool(ctx context.Context, tool contract.Tool, id string, fields map[string]any) (string, error) {
	arguments, err := json.Marshal(fields)
	if err != nil {
		return "", err
	}
	output, err := running.underTheTimeLimit(ctx, tool, contract.ToolCall{ID: "done-check-" + id, Name: tool.Spec().Name, Input: arguments})
	if err != nil {
		return "", err
	}
	return output.Text, nil
}

// theAnswerIn reads the page's answer to the overflow ask off the read
// tool's text, and says whether there was one.
func theAnswerIn(text string) (int, bool) {
	for _, line := range strings.Split(text, "\n") {
		answer, found := strings.CutPrefix(strings.TrimSpace(line), "the page answered: ")
		if !found {
			continue
		}
		pixels, err := strconv.Atoi(strings.TrimSpace(strings.Trim(answer, `"`)))
		if err != nil {
			return 0, false
		}
		return pixels, true
	}
	return 0, false
}

// theFirstPageErrorIn reads the first line under "page errors:" off the read
// tool's text, or nothing when the page had none.
func theFirstPageErrorIn(text string) string {
	lines := strings.Split(text, "\n")
	for at, line := range lines {
		if strings.TrimSpace(line) != "page errors:" || at+1 >= len(lines) {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[at+1]), "- "))
	}
	return ""
}

// joinWidths writes the widths the way a sentence lists them: "1440, 768 and
// 390".
func joinWidths(widths []string) string {
	if len(widths) <= 1 {
		return strings.Join(widths, "")
	}
	return strings.Join(widths[:len(widths)-1], ", ") + " and " + widths[len(widths)-1]
}
