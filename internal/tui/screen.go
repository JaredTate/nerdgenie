package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/JaredTate/coeus/internal/clock"
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
	// Commands are the slash commands the palette offers before the program has
	// reported its own list.
	Commands []contract.Command
	// Dialer opens the link to the running program. A screen built without one
	// draws its frame and never connects, which is what the drawing tests use.
	Dialer Dialer
	// Size reports how big the terminal is and whether it would say. Run uses it
	// to draw the very first frame at the real size; a caller that leaves it out
	// gets the terminal the frame is written to, asked directly.
	Size func() (width int, height int, known bool)
	// Width and Height are the size to draw the first frame at, before the
	// terminal has said how big it really is.
	Width, Height int
	// Output is where the frame is written, which is the terminal itself.
	Output io.Writer
	// Input is where key presses are read from, which is the terminal itself.
	Input io.Reader
}

// Run draws the screen on the terminal and does not return until the person
// quits or the terminal goes away.
func Run(options Options) error {
	options.Width, options.Height = firstFrameSize(options)
	screen := New(options)
	defer screen.Close()

	// The frame asks for the alternate screen itself, in View below, because
	// that is where Bubble Tea reads it from now. The size is handed over as
	// well, because a screen drawn to something that is not a terminal has no
	// size to ask for and would otherwise be drawn no columns wide.
	settings := []tea.ProgramOption{tea.WithWindowSize(options.Width, options.Height)}
	if options.Output != nil {
		settings = append(settings, tea.WithOutput(options.Output))
	}
	if options.Input != nil {
		settings = append(settings, tea.WithInput(options.Input))
	}
	if _, err := tea.NewProgram(screen, settings...).Run(); err != nil {
		return fmt.Errorf("the terminal screen stopped: %w", err)
	}
	return nil
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
	client       *Client
	events       <-chan tea.Msg

	width  int
	height int
	now    time.Time

	busySince    time.Time
	spinnerSince time.Time

	state        programState
	detail       string
	attached     bool
	everAttached bool
	modelAlias   string
	taskID       string
	taskState    string
	tokensIn     string
	tokensOut    string
	money        string
	budget       string
	budgetTask   string
	budgetNow    int
	budgetMost   int
	lastHealth   time.Time

	contextTokens int
	contextWindow int
	callStarted   time.Time
	streamed      int
	lastRecord    string
	lastTool      string
	plan          string
	jobs          string

	blocks     []block
	scrollBack int
	pending    string
	streaming  bool

	input        editor
	secretID     string
	secretPrompt string
	askingWhyNot bool
	history      []string
	historyAt    int

	commands    []contract.Command
	paletteOpen bool
	quitArmed   bool

	cardsShown int
	waitingFor int
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
	reading := options.Clock
	if reading == nil {
		reading = clock.System()
	}
	screen := &Screen{
		clock:        reading,
		colors:       newTheme(environment),
		canDrawEmoji: canDrawEmoji(environment),
		pictures:     detectPictures(environment),
		link:         noLink{},
		width:        max(options.Width, smallestWidth),
		height:       max(options.Height, smallestHeight),
		now:          reading.Now(),
		state:        stateConnecting,
		commands:     options.Commands,
	}
	if options.Dialer != nil {
		screen.client = NewClient(options.Dialer, reading)
		screen.link = screen.client
	}
	return screen
}

// Init is what Bubble Tea runs once the screen is on the terminal. It starts the
// heartbeat and nothing else, because the first frame must not wait on anything.
func (screen *Screen) Init() tea.Cmd {
	if screen.client == nil {
		return screen.nextTick()
	}
	screen.events = screen.client.Start()
	return tea.Batch(screen.nextTick(), screen.listen())
}

// listen is the command that waits for the next thing the link has to say. Every
// message re-arms it, which is how one stream becomes a run of Bubble Tea
// messages.
func (screen *Screen) listen() tea.Cmd {
	if screen.events == nil {
		return nil
	}
	return func() tea.Msg {
		message, more := <-screen.events
		if !more {
			return nil
		}
		return message
	}
}

// Close ends the link to the running program. Bubble Tea has no place to do
// this, so whoever started the screen does it when the program is over.
func (screen *Screen) Close() {
	if screen.client != nil {
		screen.client.Close()
	}
}

// linkChanged takes the link coming up or going down. The status strip says so
// at once, and the input box stays usable either way, so that the person can
// still read back what happened.
func (screen *Screen) linkChanged(change linkMessage) {
	if change.up {
		screen.attached = true
		screen.everAttached = true
		screen.lastHealth = screen.now
		screen.setState(stateIdle, "")
		return
	}
	if screen.attached && change.detail != "" {
		screen.showTrouble("The link to the running program went away: " + change.detail + ". The screen will keep trying.")
	}
	screen.attached = false
	screen.setState(stateDisconnected, change.detail)
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
	screen.flushDeltas()
	screen.judgeSpinner()
}

// Update takes one thing that happened and changes the screen to match.
func (screen *Screen) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := message.(type) {
	case tea.WindowSizeMsg:
		screen.resize(typed.Width, typed.Height)
	case tea.KeyPressMsg:
		return screen, screen.pressed(typed)
	case tea.PasteMsg:
		screen.pasted(typed.Content)
	case tea.MouseWheelMsg:
		screen.wheeled(typed)
	case tickMessage:
		screen.beat(typed.at)
		return screen, screen.nextTick()
	case envelopeMessage:
		screen.receive(typed.envelope)
		return screen, screen.listen()
	case linkMessage:
		screen.linkChanged(typed)
		return screen, screen.listen()
	}
	return screen, nil
}

// View is what Bubble Tea puts on the terminal: the frame below, on the
// alternate screen, so that the person's shell is still there when they quit,
// with the mouse reported cell by cell, so that the terminal sends the wheel.
// Bubble Tea turns both off again when the screen quits, which leaves the
// shell as it was found.
func (screen *Screen) View() tea.View {
	shown := tea.NewView(screen.frame())
	shown.AltScreen = true
	shown.MouseMode = tea.MouseModeCellMotion
	return shown
}

// frame draws the whole frame: the header, a rule, the transcript, a rule, the
// input box, and the status strip, in exactly the number of rows the terminal
// has.
func (screen *Screen) frame() string {
	input := screen.inputRows()
	spare := screen.height - 4 - len(input)
	for spare < 0 && len(input) > 1 {
		input = input[1:]
		spare++
	}

	palette := screen.paletteRows()
	if len(palette) > spare {
		palette = palette[:max(spare, 0)]
	}

	middle := append(screen.visibleTranscript(spare-len(palette)), palette...)
	rows := []string{screen.headerRow(), screen.ruleRow()}
	rows = append(rows, screen.besideThePanel(middle)...)
	rows = append(rows, screen.ruleRow())
	rows = append(rows, input...)
	rows = append(rows, screen.statusRow())
	for at, drawn := range rows {
		rows[at] = screen.paintToTheEdge(drawn)
	}
	return strings.Join(rows, "\n")
}

// paintToTheEdge fills the rest of a row out to the right-hand edge of the
// terminal, so that the ground the frame is drawn on runs the whole width of
// every row and there are no unpainted gaps down the right-hand side.
func (screen *Screen) paintToTheEdge(drawn string) string {
	return screen.paintTo(drawn, screen.width)
}

// paintTo fills a row out to a number of columns with the ground, which is what
// puts the side panel at the same column on every row however short the row
// beside it is.
func (screen *Screen) paintTo(drawn string, columns int) string {
	gap := columns - displayWidth(drawn)
	if gap <= 0 {
		return drawn
	}
	return drawn + screen.colors.wrap(styleNormal, strings.Repeat(" ", gap))
}

// resize takes the terminal's new size, which re-wraps everything on the next
// frame because the frame is drawn from the blocks rather than from lines.
func (screen *Screen) resize(width int, height int) {
	screen.width = max(width, smallestWidth)
	screen.height = max(height, smallestHeight)
}

// focusedCard is the card waiting for an answer, or nothing when the keys belong
// to the input box. The card is found by the number it was given when it was
// shown rather than by walking back through the transcript, because anything at
// all may arrive above it, and an error card that landed on top of a preview
// used to take the preview's three answers away with it.
func (screen *Screen) focusedCard() *card {
	if screen.waitingFor == 0 {
		return nil
	}
	for at := len(screen.blocks) - 1; at >= 0; at-- {
		if screen.blocks[at].kind != blockCard || screen.blocks[at].shown.number != screen.waitingFor {
			continue
		}
		if screen.blocks[at].shown.takesKeys() {
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
