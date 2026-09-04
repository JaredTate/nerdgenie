// The reconnect design was borrowed from OpenClaw's stream loop at
// ~/Code/openclaw/extensions/signal/src/sse-reconnect.ts, the two-to-sixty-second
// wait and the frame rules from ZeroClaw's Signal channel at
// ~/Code/zeroclaw/crates/zeroclaw-channels/src/signal.rs, and the idea of
// forcing a reconnect when the stream has been quiet too long from Hermes'
// health monitor at ~/Code/hermes-agent/gateway/platforms/signal.py. The Go here
// is written fresh, and unlike any of them it waits on the injected clock, so a
// test never waits on a real one.

package signal

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

const (
	// eventsPath is where the daemon streams inbound messages.
	eventsPath = "/api/v1/events"
	// ReconnectMinimumWait is how long the stream waits before its first try
	// after a connection ends.
	ReconnectMinimumWait = 2 * time.Second
	// ReconnectMaximumWait is the longest the stream ever waits between tries.
	ReconnectMaximumWait = 60 * time.Second
	// SilenceBeforeReconnect is how long a stream may say nothing at all before
	// it is dropped and opened again. A daemon whose connection is dead often
	// leaves the socket open, and nothing but this notices.
	SilenceBeforeReconnect = 2 * time.Minute
	// streamLineBacklog is how many lines may wait to be read before the reader
	// stops pulling more off the socket.
	streamLineBacklog = 64
)

// Stream reads one daemon's event stream and hands every inbound message to the
// function it was given, opening the stream again whenever it drops or goes
// quiet.
type Stream struct {
	client      *Client
	clock       contract.Clock
	requests    *http.Client
	connections atomic.Int64
}

// NewStream builds a reader for the daemon the client talks to. It reads
// nothing until Run is called.
func NewStream(client *Client, clock contract.Clock) *Stream {
	return &Stream{
		client: client,
		clock:  clock,
		// The stream is meant to stay open for hours, so it gets a requester
		// with no deadline of its own; silence is noticed by the clock instead.
		requests: &http.Client{},
	}
}

// Connections is how many times the stream has been opened, which is how a
// caller or a test can see that it dropped and came back.
func (stream *Stream) Connections() int {
	return int(stream.connections.Load())
}

// Run reads the stream until the context is cancelled, opening it again after
// every drop with a wait that grows from two seconds to sixty. It returns the
// context's error, because the only way out is being told to stop.
func (stream *Stream) Run(ctx context.Context, onEvent func(Event)) error {
	wait := ReconnectMinimumWait
	for ctx.Err() == nil {
		body, err := stream.open(ctx)
		if err == nil {
			wait = ReconnectMinimumWait
			if stream.read(ctx, body, onEvent) {
				continue
			}
		}
		if ctx.Err() != nil {
			break
		}
		if err := stream.clock.Sleep(ctx, wait); err != nil {
			break
		}
		wait = nextWait(wait)
	}
	return ctx.Err()
}

// open asks the daemon for its event stream and hands back the body to read.
func (stream *Stream) open(ctx context.Context) (io.ReadCloser, error) {
	address := stream.client.baseAddress + eventsPath
	if stream.client.account != "" {
		address += "?account=" + url.QueryEscape(stream.client.account)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot ask signal-cli for its event stream at %s: %w", address, err)
	}
	request.Header.Set("Accept", "text/event-stream")

	answer, err := stream.requests.Do(request)
	if err != nil {
		return nil, fmt.Errorf("signal-cli did not open its event stream, so check that it is still running: %w", err)
	}
	if answer.StatusCode < 200 || answer.StatusCode >= 300 {
		answer.Body.Close()
		return nil, fmt.Errorf("signal-cli answered the event stream with %s, so look at its log", answer.Status)
	}
	return answer.Body, nil
}

// read carries one open stream until it ends. It returns true when the stream
// was dropped for being quiet, which is reason to open a new one at once rather
// than waiting first.
func (stream *Stream) read(ctx context.Context, body io.ReadCloser, onEvent func(Event)) bool {
	defer body.Close()
	quiet := stream.clock.NewTicker(SilenceBeforeReconnect)
	defer quiet.Stop()
	heardAt := stream.clock.Now()
	stream.connections.Add(1)

	lines := make(chan string, streamLineBacklog)
	go readLines(body, lines)

	frame := &frameReader{}
	for {
		select {
		case line, open := <-lines:
			if !open {
				return false
			}
			heardAt = stream.clock.Now()
			if payload, ready := frame.take(line); ready {
				if event, isMessage := DecodeEvent([]byte(payload)); isMessage {
					onEvent(event)
				}
			}
		case <-quiet.Ticks():
			if stream.clock.Now().Sub(heardAt) >= SilenceBeforeReconnect {
				return true
			}
		case <-ctx.Done():
			return false
		}
	}
}

// readLines pulls one line at a time off the stream and closes the channel when
// there is nothing more to read, whether the stream ended or broke.
func readLines(body io.Reader, lines chan<- string) {
	defer close(lines)
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 4096), MaxEventBytes)
	for scanner.Scan() {
		lines <- scanner.Text()
	}
}

// nextWait doubles the wait between tries, up to the longest one.
func nextWait(wait time.Duration) time.Duration {
	doubled := wait * 2
	if doubled > ReconnectMaximumWait {
		return ReconnectMaximumWait
	}
	return doubled
}

// frameReader gathers the lines of one server-sent event. A line of its own ends
// the event; a line beginning with a colon is the daemon saying it is still
// there; every other field but the data is thrown away, because the payload is
// the whole of what matters.
type frameReader struct {
	payload    strings.Builder
	overflowed bool
}

// take reads one line and returns the payload of an event when that line
// finished one.
func (reader *frameReader) take(line string) (string, bool) {
	line = strings.TrimSuffix(line, "\r")
	if line == "" {
		return reader.finish()
	}
	if strings.HasPrefix(line, ":") {
		return "", false
	}
	field, value, found := strings.Cut(line, ":")
	if !found || field != "data" {
		return "", false
	}
	value = strings.TrimPrefix(value, " ")
	if reader.payload.Len()+len(value)+1 > MaxEventBytes {
		reader.overflowed = true
		return "", false
	}
	if reader.payload.Len() > 0 {
		reader.payload.WriteString("\n")
	}
	reader.payload.WriteString(value)
	return "", false
}

// finish ends one event, handing back what was gathered unless there was too
// much of it or nothing at all.
func (reader *frameReader) finish() (string, bool) {
	payload := reader.payload.String()
	skipped := reader.overflowed
	reader.payload.Reset()
	reader.overflowed = false
	if skipped || payload == "" {
		return "", false
	}
	return payload, true
}
