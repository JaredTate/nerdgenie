package browser

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// theTestMoment is where every test's clock starts, so that the day the budget
// is counted on and the time it resets at are the same in every test.
var theTestMoment = time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)

// world is everything a test hands the browser, kept together so that a test
// says what it is about rather than how it was wired.
type world struct {
	worker  *testkit.FakeBrowserWorker
	channel *testkit.FakeChannel
	secrets *testkit.FakeSecrets
	clock   *testkit.FakeClock
	codes   *fixedCodes
	starts  *countedStarts
	notes   *noteBook
}

// newWorld returns the fakes a browser test runs against, with the fake worker
// served over a local socket so that the client is proved against
// worker/browser/PROTOCOL.md rather than against a Go interface.
func newWorld(t *testing.T) *world {
	t.Helper()
	worker := testkit.NewFakeBrowserWorker()
	server := testkit.NewBrowserProtocolServer(t, worker)
	clock := testkit.NewFakeClock(theTestMoment)
	return &world{
		worker:  worker,
		channel: testkit.NewFakeChannel(contract.TerminalChannelName),
		secrets: testkit.NewFakeSecrets(),
		clock:   clock,
		codes:   &fixedCodes{code: "123456", secondsLeft: 25},
		starts:  &countedStarts{start: socketStart(server), clock: clock},
		notes:   &noteBook{},
	}
}

// startedBy makes the counted starts wrap a different worker, so that a test can
// keep the counting and change what is being started.
func (world *world) startedBy(start Start) Start {
	world.starts.start = start
	return world.starts.run
}

// browser builds the browser under test, with the option that a test wants to
// change passed in.
func (world *world) browser(t *testing.T, change func(options *Options)) *Browser {
	t.Helper()
	options := Options{
		Start:               world.starts.run,
		Channel:             world.channel,
		Secrets:             world.secrets,
		Codes:               world.codes,
		Clock:               world.clock,
		HandoffTimeout:      time.Minute,
		DailyActionsPerSite: 50,
		IdleStop:            DefaultIdleStop,
		Note:                world.notes.write,
	}
	if change != nil {
		change(&options)
	}
	built, err := New(options)
	if err != nil {
		t.Fatalf("building the browser failed: %v", err)
	}
	t.Cleanup(func() { _ = built.Close() })
	return built
}

// openTheSimplePage puts the browser on the fixture page every acting test
// starts from.
func openTheSimplePage(t *testing.T, browser *Browser) contract.Snapshot {
	t.Helper()
	page, err := browser.Open(context.Background(), testkit.FixtureSimplePage)
	if err != nil {
		t.Fatalf("opening the fixture page failed: %v", err)
	}
	return page
}

// fixedCodes is a two-factor code maker a test controls.
type fixedCodes struct {
	guard       sync.Mutex
	code        string
	secondsLeft int
	asked       int
	problem     error
}

// Code hands back the code the test set, counting how many times it was asked
// and shortening the seconds left only when the test told it to.
func (codes *fixedCodes) Code(string) (string, int, error) {
	codes.guard.Lock()
	defer codes.guard.Unlock()
	codes.asked++
	if codes.problem != nil {
		return "", 0, codes.problem
	}
	if codes.asked > 1 {
		return codes.code, 30, nil
	}
	return codes.code, codes.secondsLeft, nil
}

// timesAsked is how many codes were made.
func (codes *fixedCodes) timesAsked() int {
	codes.guard.Lock()
	defer codes.guard.Unlock()
	return codes.asked
}

// noteBook keeps every line the browser wrote about what it did, so that a test
// can say what a person reading the log would have seen.
type noteBook struct {
	guard sync.Mutex
	lines []string
}

// write records one line.
func (book *noteBook) write(format string, arguments ...any) {
	book.guard.Lock()
	defer book.guard.Unlock()
	book.lines = append(book.lines, fmt.Sprintf(format, arguments...))
}

// written is everything that was noted, joined into one piece of text.
func (book *noteBook) written() string {
	book.guard.Lock()
	defer book.guard.Unlock()
	return fmt.Sprint(book.lines)
}

// countedStarts wraps a start function and counts the workers started and
// stopped, and when each one was started.
type countedStarts struct {
	guard   sync.Mutex
	start   Start
	started int
	stopped int
	at      []time.Time
	clock   contract.Clock
}

// run starts one worker and counts it.
func (counted *countedStarts) run(ctx context.Context) (*Connection, error) {
	counted.guard.Lock()
	counted.started++
	if counted.clock != nil {
		counted.at = append(counted.at, counted.clock.Now())
	}
	counted.guard.Unlock()

	connection, err := counted.start(ctx)
	if err != nil || connection == nil {
		return connection, err
	}
	stop := connection.Stop
	connection.Stop = func() error {
		counted.guard.Lock()
		counted.stopped++
		counted.guard.Unlock()
		if stop == nil {
			return nil
		}
		return stop()
	}
	return connection, nil
}

// counts is how many workers were started and how many were stopped.
func (counted *countedStarts) counts() (int, int) {
	counted.guard.Lock()
	defer counted.guard.Unlock()
	return counted.started, counted.stopped
}

// moments is when each worker was started, on the clock the test controls.
func (counted *countedStarts) moments() []time.Time {
	counted.guard.Lock()
	defer counted.guard.Unlock()
	return append([]time.Time(nil), counted.at...)
}

// socketStart dials the fake worker's protocol server, which speaks the same
// document the real worker speaks.
func socketStart(server *testkit.BrowserProtocolServer) Start {
	return func(context.Context) (*Connection, error) {
		connection, err := net.Dial("unix", server.SocketPath())
		if err != nil {
			return nil, fmt.Errorf("dialling the fake browser worker failed: %w", err)
		}
		return &Connection{
			Requests:  connection,
			Responses: connection,
			ProcessID: 4242,
			Stop:      connection.Close,
		}, nil
	}
}

// scriptedStart returns a worker that answers each request line with whatever
// the test says, so that answers no real worker would send can be proved too.
// An empty answer means the worker says nothing at all.
func scriptedStart(answer func(line []byte) string) Start {
	return func(context.Context) (*Connection, error) {
		toWorker, requests := io.Pipe()
		answers, fromWorker := io.Pipe()
		go serveScript(toWorker, fromWorker, answer)
		return &Connection{
			Requests:  requests,
			Responses: answers,
			ProcessID: 1234,
			Stop: func() error {
				_ = requests.Close()
				_ = answers.Close()
				return nil
			},
		}, nil
	}
}

// serveScript reads one request line at a time and writes back what the script
// says, until either pipe closes.
func serveScript(toWorker io.ReadCloser, fromWorker io.WriteCloser, answer func(line []byte) string) {
	defer func() { _ = fromWorker.Close() }()
	defer func() { _ = toWorker.Close() }()
	reader := bufio.NewReader(toWorker)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if written := answer(line); written != "" {
				if _, err := io.WriteString(fromWorker, written); err != nil {
					return
				}
			}
		}
		if err != nil {
			return
		}
	}
}

// healthyThen answers the health check the way a working worker does and hands
// every other request to the script.
func healthyThen(answer func(line []byte) string) func(line []byte) string {
	return func(line []byte) string {
		if strings.Contains(string(line), `"method":"health"`) {
			return fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{"healthy":true,"chromeVersion":"scripted"}}`+"\n", identifierIn(line))
		}
		return answer(line)
	}
}

// identifierIn reads the request identifier out of a request line, so that a
// scripted answer can echo it back the way the protocol requires.
func identifierIn(line []byte) string {
	const marker = `"id":`
	whole := string(line)
	at := strings.Index(whole, marker)
	if at < 0 {
		return "0"
	}
	end := at + len(marker)
	for end < len(whole) && whole[end] >= '0' && whole[end] <= '9' {
		end++
	}
	return whole[at+len(marker) : end]
}

// waitForSleepers waits until that many callers are waiting inside the fake
// clock's Sleep, so that a test moves time only once the code under test is
// really waiting for it.
func waitForSleepers(t *testing.T, clock *testkit.FakeClock, wanted int) {
	t.Helper()
	waitUntil(t, fmt.Sprintf("%d callers to be waiting on the clock", wanted), func() bool {
		return clock.Sleepers() >= wanted
	})
}

// waitUntil waits for something to become true, and fails the test with what it
// was waiting for when it never does.
func waitUntil(t *testing.T, what string, ready func() bool) {
	t.Helper()
	const limit = 2 * time.Second
	giveUpAt := time.Now().Add(limit)
	for time.Now().Before(giveUpAt) {
		if ready() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("waited %s for %s and it never happened", limit, what)
}

// refused is the refusal inside an error, and fails the test when the error was
// not a refusal from the worker at all.
func refused(t *testing.T, err error) *RefusedError {
	t.Helper()
	var refusal *RefusedError
	if !errors.As(err, &refusal) {
		t.Fatalf("expected a refusal from the browser worker, and got: %v", err)
	}
	return refusal
}
