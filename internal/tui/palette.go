// One command table shared by every screen, and a palette that lists it and
// filters it as the person types, is ported from OpenCode's command registry in
// ~/Code/opencode/packages/tui/src/app.tsx. The Go here is written fresh, and the
// list itself belongs to the running program rather than to this screen.

package tui

import (
	"sort"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// maxPaletteRows is how many commands the palette lists at once, so that it
// never swallows the transcript.
const maxPaletteRows = 8

// commandGlyph is the slash a command is typed with. The palette draws it in
// front of every name it lists, so a name is held without one.
const commandGlyph = "/"

// openPalette shows the command palette, which happens when the person types a
// slash as the first character in an empty box.
func (screen *Screen) openPalette() {
	screen.paletteOpen = true
}

// closePalette hides the palette and leaves what was typed alone.
func (screen *Screen) closePalette() {
	screen.paletteOpen = false
}

// judgePalette closes the palette when what is being typed is no longer a slash
// command, which is the only way it closes without the person asking.
func (screen *Screen) judgePalette() {
	if !strings.HasPrefix(screen.input.text(), "/") {
		screen.closePalette()
	}
}

// paletteMatches are the commands that still match what has been typed, in the
// order they are listed.
func (screen *Screen) paletteMatches() []contract.Command {
	typed, isCommand := slashCommand(screen.input.text())
	if !isCommand {
		typed = ""
	}
	if space := strings.IndexAny(typed, " \t"); space >= 0 {
		typed = typed[:space]
	}
	typed = strings.ToLower(typed)

	matched := []contract.Command{}
	for _, one := range screen.commands {
		if strings.HasPrefix(strings.ToLower(one.Name), typed) {
			matched = append(matched, one)
		}
	}
	if len(matched) > maxPaletteRows {
		matched = matched[:maxPaletteRows]
	}
	return matched
}

// paletteRows draws the palette: a dim list above the input box of every command
// that still matches, each with its one help line.
func (screen *Screen) paletteRows() []string {
	if !screen.paletteOpen {
		return nil
	}
	matched := screen.paletteMatches()
	if len(matched) == 0 {
		return nil
	}
	widest := 0
	for _, one := range matched {
		widest = max(widest, displayWidth(one.Name)+1)
	}
	drawn := []string{}
	for _, one := range matched {
		line := row{}
		line.blanks(marginColumns + gutterColumns)
		line.add(styleAccent, commandGlyph+one.Name)
		line.blanks(widest - displayWidth(one.Name))
		line.add(styleDim, cutTo(one.Help, screen.transcriptColumns()-marginColumns-gutterColumns-widest-2))
		drawn = append(drawn, line.render(screen.colors))
	}
	return drawn
}

// completeCommand puts the first matching command in the input box and closes
// the palette, which is what Tab and Enter do while it is open.
func (screen *Screen) completeCommand() bool {
	matched := screen.paletteMatches()
	if len(matched) == 0 {
		return false
	}
	screen.input.setText(commandGlyph + matched[0].Name + " ")
	screen.closePalette()
	return true
}

// learnCommands takes the command list the program reported and puts it in the
// palette, sorted by name so that the list reads the same every time. The
// program writes each name with the slash it is typed with, and the palette
// draws a slash of its own and matches on what is typed after one, so the slash
// is taken off here. Left on, every command was drawn as "//help" and nothing
// matched the moment a letter was typed after the slash.
func (screen *Screen) learnCommands(listed string) {
	learned := []contract.Command{}
	for _, line := range strings.Split(listed, "\n") {
		name, help, _ := strings.Cut(strings.TrimSpace(line), contract.StatusCommandSeparator)
		name = strings.TrimPrefix(name, commandGlyph)
		if name == "" {
			continue
		}
		learned = append(learned, contract.Command{Name: name, Help: strings.TrimSpace(help)})
	}
	if len(learned) == 0 {
		return
	}
	sort.Slice(learned, func(left, right int) bool { return learned[left].Name < learned[right].Name })
	screen.commands = learned
}
