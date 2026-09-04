package command

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// Session is one conversation. A session holds the tasks the user ran in it, so
// "/new" starts a fresh one and "/sessions" lists them and switches between
// them.
type Session struct {
	// ID is what "/sessions 12" names, such as "12".
	ID string
	// Title is the one line saying what the conversation was about.
	Title string
	// Started is when the first message in it arrived.
	Started time.Time
	// Current says this is the session the user is talking in.
	Current bool
}

// Deps is everything the core commands need from the rest of the program. Each
// field is a function or a store that cmd/coeus/serve.go fills in, so this
// package never imports the loop, the channels, or the terminal screen, and a
// test can hand every command a fake.
//
// A field left empty is not a crash: the command that needed it answers with a
// sentence naming the field serve.go has to fill.
type Deps struct {
	// Settings is the configuration the program loaded, which "/model" checks an
	// alias against.
	Settings contract.Config
	// Store is the event log, which "/undo" reads the last turn's file changes
	// from.
	Store contract.Store
	// Jobs is the job store, which "/pause" lists and pauses through.
	Jobs contract.Job
	// ResumeJob starts one paused job running again. It is a function of its own
	// because contract.Job has no way to undo a pause.
	ResumeJob func(ctx context.Context, jobID string) error
	// CurrentModel is the alias in use for this session.
	CurrentModel func() string
	// SetModel makes an alias the one in use for this session.
	SetModel func(alias string) error
	// CostSoFar is what the session has spent so far, which "/status" prints.
	CostSoFar func() contract.CostLine
	// PendingPreviews is every preview or question waiting for an answer.
	PendingPreviews func(ctx context.Context) ([]contract.Preview, error)
	// Answer answers one waiting preview by its id.
	Answer func(ctx context.Context, previewID string, answer contract.PreviewAnswer, reason string) error
	// Channels is every channel the program is listening on, which "/status"
	// asks the health of.
	Channels func() []contract.Channel
	// YoloIsOn says whether the yolo switch is on, which "/status" reports,
	// because while it is on every call that would have asked runs without
	// asking. Left empty, the status says nothing about it.
	YoloIsOn func() bool
	// Sessions lists the conversations, newest first.
	Sessions func(ctx context.Context) ([]Session, error)
	// NewSession starts a fresh conversation and returns its id.
	NewSession func(ctx context.Context) (string, error)
	// SwitchSession makes one session the one the user is talking in.
	SwitchSession func(ctx context.Context, sessionID string) error
}

// Commands is the set of core slash commands, built once against one Deps.
// It holds the little state two commands share: which jobs "/pause" paused, so
// that "/resume" starts exactly those again and nothing else.
type Commands struct {
	registry *Registry
	deps     Deps

	guard  sync.Mutex
	paused []string
}

// New builds the core commands against the registry they will be registered in,
// which "/help" reads to print its listing.
func New(registry *Registry, deps Deps) *Commands {
	return &Commands{registry: registry, deps: deps}
}

// All returns the ten core commands in the order the help listing prints them.
// cmd/coeus/serve.go registers every one of them.
func (commands *Commands) All() []contract.Command {
	return []contract.Command{
		commands.Help(),
		commands.Status(),
		commands.Model(),
		commands.NewSession(),
		commands.Sessions(),
		commands.Approve(),
		commands.Deny(),
		commands.Pause(),
		commands.Resume(),
		commands.Undo(),
	}
}

// notWiredUp is the error a command answers with when the piece of the program
// it needed was never filled in. It names the field, because the reader who
// sees it is the one editing serve.go.
func notWiredUp(name string, field string) error {
	return fmt.Errorf("the /%s command has nothing to work with, so fill Deps.%s in cmd/coeus/serve.go", name, field)
}
