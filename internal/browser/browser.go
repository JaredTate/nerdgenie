package browser

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// The bounds this package keeps, because every wait has a timeout and every
// retry has a limit.
const (
	// DefaultIdleStop is how long a worker with nothing to do is kept before it
	// is stopped and its window closed.
	DefaultIdleStop = 30 * time.Minute
	// DefaultDailyActionsPerSite is how many actions one hostname gets in a day
	// when the caller names no number. The configuration has no field for this
	// yet, so this is the number every install uses.
	DefaultDailyActionsPerSite = 300
	// DefaultHandoffTimeout is how long a handoff waits for the user when the
	// caller names no timeout, and is the default in contract.DefaultConfig.
	DefaultHandoffTimeout = 30 * time.Minute
	// idleCheckEvery is how often the idle watcher looks at how long it has been
	// since the worker was last used.
	idleCheckEvery = time.Minute
	// firstRestartWait is how long to wait before trying again after a worker
	// would not start.
	firstRestartWait = time.Second
	// longestRestartWait caps that wait, so that the backoff cannot grow without
	// end.
	longestRestartWait = 30 * time.Second
	// mostStartAttempts bounds how many times one call will try to start a
	// worker before it gives up and says so.
	mostStartAttempts = 4
)

// Pacing is how fast the worker acts on a page.
type Pacing string

const (
	// PacingHuman moves the mouse along a curve, holds a click for a human
	// length of time, and types one key at a time, which is what a real site
	// sees and what keeps the user's accounts safe.
	PacingHuman Pacing = "human"
	// PacingFast takes every wait out. Only a test asks for it.
	PacingFast Pacing = "fast"
)

// Start starts one browser worker. The running program passes ProcessStart; a
// test passes a worker of its own on a pair of pipes or a socket.
type Start func(ctx context.Context) (*Connection, error)

// TwoFactorCodes makes the two-factor code for one vault entry and says how many
// seconds are left before it changes. The running program passes the vault,
// whose Code method is exactly this; a test passes one of its own.
type TwoFactorCodes interface {
	// Code is the six-digit code for the entry with that name, and the seconds
	// left before it turns over.
	Code(name string) (string, int, error)
}

// Options is everything the Go side of the browser needs.
type Options struct {
	// Start starts one worker.
	Start Start
	// Channel is where a handoff and a screenshot are sent.
	Channel contract.Channel
	// Secrets is the vault the login credentials come from.
	Secrets contract.Secrets
	// Codes makes the two-factor code for a login, and may be left out when no
	// vault entry has a two-factor secret.
	Codes TwoFactorCodes
	// Clock is where this package reads the time, so that a test controls it.
	Clock contract.Clock
	// HandoffTimeout is how long a handoff waits for the user. Zero means
	// DefaultHandoffTimeout.
	HandoffTimeout time.Duration
	// DailyActionsPerSite is how many actions one hostname gets in a day. Zero
	// means DefaultDailyActionsPerSite.
	DailyActionsPerSite int
	// IdleStop is how long a worker with nothing to do is kept. Zero means
	// DefaultIdleStop.
	IdleStop time.Duration
	// BufferedEvents is how many of the person's own events are held for a
	// reader that has fallen behind. Zero means DefaultBufferedEvents, which is
	// the cap the design gives in contract.Caps.
	BufferedEvents int
	// Note writes one line about what happened, and may be left out.
	Note func(format string, arguments ...any)
}

// Browser is the Go side of the browser worker. It implements
// contract.BrowserWorker, keeps one worker alive, counts each site's actions
// against its daily budget, logs in from the vault, and hands the window to the
// user when it meets a wall.
type Browser struct {
	options    Options
	budget     *dailyBudget
	guard      sync.Mutex
	connection *Connection
	talker     *client
	lastUsed   time.Time
	stopWatch  context.CancelFunc
	events     *eventStream
	page       string
	closed     bool
}

// Browser answers every method in worker/browser/PROTOCOL.md, which is what
// contract.BrowserWorker names.
var _ contract.BrowserWorker = (*Browser)(nil)

// New builds the Go side of the browser, and refuses options with a piece
// missing.
func New(options Options) (*Browser, error) {
	switch {
	case options.Start == nil:
		return nil, errors.New("the browser needs a start function to start its worker with, and none was given")
	case options.Channel == nil:
		return nil, errors.New("the browser needs a channel to hand the window to the user through, and none was given")
	case options.Secrets == nil:
		return nil, errors.New("the browser needs the vault to log in from, and none was given")
	case options.Clock == nil:
		return nil, errors.New("the browser needs a clock to read the time from, and none was given")
	}
	if options.Note == nil {
		options.Note = func(string, ...any) {}
	}
	if options.HandoffTimeout <= 0 {
		options.HandoffTimeout = DefaultHandoffTimeout
	}
	if options.DailyActionsPerSite <= 0 {
		options.DailyActionsPerSite = DefaultDailyActionsPerSite
	}
	if options.IdleStop <= 0 {
		options.IdleStop = DefaultIdleStop
	}
	if options.BufferedEvents <= 0 {
		options.BufferedEvents = DefaultBufferedEvents
	}
	return &Browser{
		options: options,
		budget:  newDailyBudget(options.Clock, options.DailyActionsPerSite),
	}, nil
}

// Close stops the worker and closes the browser window. Calling it twice is
// harmless, and nothing starts a worker again afterwards.
func (browser *Browser) Close() error {
	browser.guard.Lock()
	defer browser.guard.Unlock()
	browser.closed = true
	return browser.stopWorker()
}

// Running says whether a worker is up, which is what "/status" reports and what
// tells a test that the idle watcher has done its work.
func (browser *Browser) Running() bool {
	browser.guard.Lock()
	defer browser.guard.Unlock()
	return browser.connection != nil
}

// ProcessID is the exact process identifier of the running worker, and zero when
// there is none.
func (browser *Browser) ProcessID() int {
	browser.guard.Lock()
	defer browser.guard.Unlock()
	return browser.workerProcessID()
}

// CurrentAddress is the address of the page the worker is on, as far as the last
// answer said, and empty when nothing has been opened.
func (browser *Browser) CurrentAddress() string {
	browser.guard.Lock()
	defer browser.guard.Unlock()
	return browser.page
}

// rememberPage keeps the address of the page the worker is on, which is what the
// daily budget is counted against and what a login checks its domains against.
func (browser *Browser) rememberPage(address string) {
	if address == "" {
		return
	}
	browser.guard.Lock()
	defer browser.guard.Unlock()
	browser.page = address
}

// hostnameNow is the hostname of the page the worker is on, and empty when it is
// on none or the address has no hostname.
func (browser *Browser) hostnameNow() string {
	return hostnameOf(browser.CurrentAddress())
}

// hostnameOf reads the hostname out of an address, with the leading "www." left
// off so that one site is one entry in the budget.
func hostnameOf(address string) string {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	host := parsed.Hostname()
	if after, found := trimLeadingWorldWideWeb(host); found {
		return after
	}
	return host
}

// trimLeadingWorldWideWeb takes the "www." off a hostname and says whether there
// was one.
func trimLeadingWorldWideWeb(host string) (string, bool) {
	const prefix = "www."
	if len(host) > len(prefix) && host[:len(prefix)] == prefix {
		return host[len(prefix):], true
	}
	return "", false
}
