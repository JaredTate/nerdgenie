package loop

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/workorder"
)

// The harness takes the photograph. The sky task of the flight-simulator work
// order spent hours steering the aircraft for a distinct photograph of each
// element and then doubting the photograph, because a done line that says
// "seen and photographed" was proved only by the model's own picture and its
// own judgment of it. A done line whose text holds the word photograph or
// screenshot is now a line the harness proves itself, through the same
// browser tools the model has: browser_resize at each width the line names,
// at most two, then browser_screenshot, the way the looks check goes
// (lookscheck.go). The screenshot tool's own line on how many frames the
// page drew in a quarter of a second says whether the picture proves
// anything: a page that drew none has stopped or is hidden, and its picture
// proves nothing. With no browser tool wired, the line is the model's to
// prove, as it was.

// MaxPhotographWidths is how many widths one done line is photographed at:
// two, a desktop and a phone, because a line that names more is a looks
// check's job.
const MaxPhotographWidths = 2

// TheNoFramesRefusal is what the model reads when the page drew no frames
// for the harness's picture.
const TheNoFramesRefusal = "the harness photographed the page for this line, but the page drew no frames, so the picture proves nothing; make the page draw, then close"

// ThePageDrewWords open the screenshot tool's line on the page's frames, and
// TheFramesInAQuarterSecondWords close it: "the page drew 60 frames in a
// quarter of a second", or "no" in place of the number.
const (
	ThePageDrewWords               = "the page drew "
	TheFramesInAQuarterSecondWords = " frames in a quarter of a second"
)

// theWordsOfAPhotograph are the words a done line says to be proved by a
// picture, matched inside "photographed" and "screenshots" too.
var theWordsOfAPhotograph = []string{"photograph", "screenshot"}

// theWidthInALine finds a width a done line names, in the shape "at 1440" or
// "1440 wide"; three or four digits, because a smaller number is not a width.
var theWidthInALine = regexp.MustCompile(`\bat (\d{3,4})\b|\b(\d{3,4}) wide\b`)

// photograph is what the harness's camera came back with for one done line:
// what each picture said, in one text, one clause on each for the summary,
// and the fewest frames any of the pictures drew.
type photograph struct {
	seen         string
	said         []string
	fewestFrames int
}

// photographTheLines takes the harness's own picture for every done line that
// says photograph or screenshot and pins the line to the picture's result
// when the page drew frames. It hands back the refusal for the first line
// whose page drew none, or nothing when every such line is proved or left to
// the model.
func (running *run) photographTheLines(ctx context.Context) (string, error) {
	for at, line := range running.keeper.Record().Goal.DoneWhen {
		if _, checked := workorder.ReadCheck(line.Text); checked || !saysPhotograph(line.Text) {
			continue
		}
		taken, could := running.takeThePhotograph(ctx, line.Text)
		if !could {
			continue
		}
		if taken.fewestFrames == 0 {
			return fmt.Sprintf("The done line %q is not proved: %s", line.Text, TheNoFramesRefusal), nil
		}
		if err := running.pinTheLineToThePhotograph(ctx, at+1, taken); err != nil {
			return "", err
		}
	}
	return "", nil
}

// saysPhotograph says whether a done line asks to be proved by a picture.
func saysPhotograph(text string) bool {
	lowered := strings.ToLower(text)
	for _, word := range theWordsOfAPhotograph {
		if strings.Contains(lowered, word) {
			return true
		}
	}
	return false
}

// takeThePhotograph photographs the page for one done line, at each width
// the line names through browser_resize first, or once at the page's current
// size when it names none, and says whether it could. A browser tool that is
// not wired, a picture that could not be taken, or a camera that does not
// say how many frames the page drew leaves the line to the model's proof.
func (running *run) takeThePhotograph(ctx context.Context, text string) (photograph, bool) {
	shot, found := running.tools().Lookup(contract.ToolBrowserScreenshot)
	if !found {
		return photograph{}, false
	}
	widths := theWidthsIn(text)
	resize, found := running.tools().Lookup(contract.ToolBrowserResize)
	if len(widths) > 0 && !found {
		return photograph{}, false
	}
	// A width of zero is the page's current size, which is where a line that
	// names no width is photographed.
	sizes := widths
	if len(sizes) == 0 {
		sizes = []int{0}
	}
	taken := photograph{}
	frames := []int{}
	for _, width := range sizes {
		intent := "the done check photographs the page for the line: " + text
		if width > 0 {
			intent = fmt.Sprintf("the done check photographs the page %d wide for the line: %s", width, text)
			if _, err := running.useTheTool(ctx, resize, "photograph-resize", map[string]any{"intent": intent, "width": width, "height": TheLooksHeight}); err != nil {
				return photograph{}, false
			}
		}
		seen, err := running.useTheTool(ctx, shot, "photograph-shot", map[string]any{"intent": intent})
		if err != nil {
			return photograph{}, false
		}
		drew, known := theFramesDrawnIn(seen)
		if !known {
			return photograph{}, false
		}
		frames = append(frames, drew)
		taken.seen += seen + "\n"
		taken.said = append(taken.said, theSizeClause(width)+", "+ThePageDrewWords+theFramesWord(drew)+TheFramesInAQuarterSecondWords)
	}
	taken.fewestFrames = slices.Min(frames)
	return taken, true
}

// pinTheLineToThePhotograph writes the pictures into the record as one result
// and points the done line at it.
func (running *run) pinTheLineToThePhotograph(ctx context.Context, number int, taken photograph) error {
	summary := "photograph: " + strings.Join(taken.said, "; ")
	label, err := running.keeper.AddResult(ctx, summary, summary+"\n"+taken.seen)
	if err != nil {
		return fmt.Errorf("cannot write the photograph of done line %d into the record: %w", number, err)
	}
	return running.markTheDoneLine(ctx, number, label)
}

// theWidthsIn reads the widths a done line names, in the line's order, at
// most MaxPhotographWidths of them.
func theWidthsIn(text string) []int {
	widths := []int{}
	for _, match := range theWidthInALine.FindAllStringSubmatch(text, MaxPhotographWidths) {
		written := match[1] + match[2]
		if width, err := strconv.Atoi(written); err == nil {
			widths = append(widths, width)
		}
	}
	return widths
}

// theFramesDrawnIn reads, off the screenshot tool's text, how many frames the
// page drew in a quarter of a second: the number between "the page drew" and
// "frames in a quarter of a second", or zero for "no". It says whether the
// line was there at all, because a camera that does not count frames cannot
// say whether the page is alive.
func theFramesDrawnIn(text string) (int, bool) {
	_, after, found := strings.Cut(text, ThePageDrewWords)
	if !found {
		return 0, false
	}
	word, rest, _ := strings.Cut(after, " ")
	if !strings.HasPrefix(" "+rest, TheFramesInAQuarterSecondWords) {
		return 0, false
	}
	if word == "no" {
		return 0, true
	}
	frames, err := strconv.Atoi(word)
	if err != nil {
		return 0, false
	}
	return frames, true
}

// theSizeClause names the size a picture was taken at, for the summary.
func theSizeClause(width int) string {
	if width == 0 {
		return "at the page's size"
	}
	return fmt.Sprintf("at %d", width)
}

// theFramesWord writes the frame count the way the camera does, "no" for
// none.
func theFramesWord(frames int) string {
	if frames == 0 {
		return "no"
	}
	return strconv.Itoa(frames)
}
