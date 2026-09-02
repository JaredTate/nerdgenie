package tui

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JaredTate/coeus/internal/contract"
)

// sender is where the screen puts the envelopes it wants the running program to
// see. The link over the socket is one, and a test's recorder is another.
type sender interface {
	// Send hands one envelope to the program, or says why it could not.
	Send(envelope contract.SocketEnvelope) error
}

// noLink stands in for the link before there is one, so that nothing in the
// screen has to ask whether it has a link before it speaks.
type noLink struct{}

// Send always fails, because there is nothing on the other end yet.
func (noLink) Send(_ contract.SocketEnvelope) error {
	return errors.New("there is no link to the running program yet, so start it with \"coeus serve\" and this screen will attach on its own")
}

// Options is everything the screen needs that it cannot work out for itself.
type Options struct {
	// Clock is where the screen reads the time, so that a test can control it.
	Clock contract.Clock
	// Environment is how the screen reads the terminal's settings, which is
	// os.Getenv outside a test.
	Environment func(name string) string
	// Commands are the slash commands the palette offers.
	Commands []contract.Command
	// Width and Height are the size to draw the first frame at, before the
	// terminal has said how big it really is.
	Width, Height int
}

// Screen is the terminal screen: one Bubble Tea model holding what is on the
// frame and nothing else. Everything it draws came from the running program, and
// everything the person types goes straight back to it.
type Screen struct {
	clock        contract.Clock
	colors       theme
	canDrawEmoji bool
	pictures     pictureProtocol
	link         sender

	width  int
	height int
	now    time.Time

	busySince    time.Time
	spinnerSince time.Time

	state      programState
	detail     string
	attached   bool
	modelAlias string
	taskID     string
	taskState  string
	tokensIn   string
	tokensOut  string
	money      string
	budget     string
	lastHealth time.Time

	blocks     []block
	scrollBack int

	input        editor
	secretPrompt string
	askingWhyNot bool
	history      []string
	historyAt    int

	commands    []contract.Command
	paletteOpen bool
	quitArmed   bool
}

// heartbeatInterval is how often the screen wakes itself up. It is the thirty
// milliseconds docs/TUI_DESIGN.md gives for coalescing streamed text, which is
// also often enough for the spinner's eight frames a second.
const heartbeatInterval = 30 * time.Millisecond

// New builds a screen ready to draw its first frame. Nothing here waits on the
// network, which is what lets the first frame appear before the link is made.
func New(options Options) *Screen {
	environment := options.Environment
	if environment == nil {
		environment = os.Getenv
	}
	clock := options.Clock
	if clock == nil {
		clock = systemClock{}
	}
	screen := &Screen{
		clock:        clock,
		colors:       newTheme(environment),
		canDrawEmoji: canDrawEmoji(environment),
		pictures:     detectPictures(environment),
		link:         noLink{},
		width:        max(options.Width, smallestWidth),
		height:       max(options.Height, smallestHeight),
		now:          clock.Now(),
		state:        stateConnecting,
		commands:     options.Commands,
	}
	screen.historyAt = 0
	return screen
}

// Init is what Bubble Tea runs once the screen is on the terminal. It starts the
// heartbeat and nothing else, because the first frame must not wait on anything.
func (screen *Screen) Init() tea.Cmd {
	return screen.nextTick()
}

// nextTick is the command that brings the screen its next heartbeat, one
// interval from now on contract.Clock.
func (screen *Screen) nextTick() tea.Cmd {
	return func() tea.Msg {
		if err := screen.clock.Sleep(context.Background(), heartbeatInterval); err != nil {
			return nil
		}
		return tickMessage{at: screen.clock.Now()}
	}
}

// beat takes one heartbeat: it moves the screen's idea of the time on and lets
// the spinner decide whether it is still due.
func (screen *Screen) beat(at time.Time) {
	screen.now = at
	screen.judgeSpinner()
}

// Update takes one thing that happened and changes the screen to match.
func (screen *Screen) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := message.(type) {
	case tea.WindowSizeMsg:
		screen.resize(typed.Width, typed.Height)
	case tea.KeyMsg:
		return screen, screen.pressed(typed)
	case tickMessage:
		screen.beat(typed.at)
		return screen, screen.nextTick()
	}
	return screen, nil
}

// View draws the whole frame: the header, a rule, the transcript, a rule, the
// input box, and the status strip, in exactly the number of rows the terminal
// has.
func (screen *Screen) View() string {
	input := screen.inputRows()
	spare := screen.height - 4 - len(input)
	for spare < 0 && len(input) > 1 {
		input = input[1:]
		spare++
	}

	rows := []string{screen.headerRow(), screen.ruleRow()}
	rows = append(rows, screen.visibleTranscript(spare)...)
	rows = append(rows, screen.ruleRow())
	rows = append(rows, input...)
	rows = append(rows, screen.statusRow())
	return strings.Join(rows, "\n")
}

// resize takes the terminal's new size, which re-wraps everything on the next
// frame because the frame is drawn from the blocks rather than from lines.
func (screen *Screen) resize(width int, height int) {
	screen.width = max(width, smallestWidth)
	screen.height = max(height, smallestHeight)
}

// focusedCard is the card waiting for an answer, or nothing when the keys belong
// to the input box.
func (screen *Screen) focusedCard() *card {
	for at := len(screen.blocks) - 1; at >= 0; at-- {
		if screen.blocks[at].kind != blockCard {
			continue
		}
		if screen.blocks[at].shown.waiting() {
			return &screen.blocks[at].shown
		}
		return nil
	}
	return nil
}

// showTrouble puts one error card in the transcript. Everything that can go
// wrong on this side of the socket ends up here, because a screen that crashes
// tells the person nothing.
func (screen *Screen) showTrouble(what string) {
	screen.remember(block{kind: blockCard, shown: card{
		kind:     cardError,
		title:    errorTitle,
		body:     what,
		answered: true,
	}})
}
