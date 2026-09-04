// The shape of this client was borrowed from OpenClaw's Signal client at
// ~/Code/openclaw/extensions/signal/src/client.ts, which is where the rule that
// a 201 answer is a success with no body comes from, and from Hermes' Signal
// platform at ~/Code/hermes-agent/gateway/platforms/signal.py, which is where
// the shape of the calls comes from. The three paths, the method names and the
// parameter names were then checked against signal-cli 0.13.23 itself: its
// HttpServerHandler serves only /api/v1/rpc, /api/v1/events and /api/v1/check,
// and its getAttachment answers with base64 under a "data" field. The Go here is
// written fresh.

package signal

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

const (
	// healthPath is where the daemon says whether it is alive.
	healthPath = "/api/v1/check"
	// remoteProcedurePath is where sends, typing indicators and downloads go.
	remoteProcedurePath = "/api/v1/rpc"
	// callTimeout is how long one ordinary call to the daemon may take.
	callTimeout = 30 * time.Second
	// healthTimeout is how long the health check may take, which is short
	// because a health check that hangs is a health check that failed.
	healthTimeout = 10 * time.Second
	// maxAnswerBytes caps how much of the daemon's answer is read for an
	// ordinary call. A send answers with a timestamp, so this is generous.
	maxAnswerBytes = 1 << 20
	// maxAttachmentBytes is the largest attachment that will be downloaded, which
	// is what Signal itself allows one message to carry.
	maxAttachmentBytes = 100 << 20
	// maxAttachmentAnswerBytes caps how much of an answer is read at all. It is
	// the attachment cap with room for base64, which makes a file about a third
	// bigger, and a little more for the JSON around it.
	maxAttachmentAnswerBytes = maxAttachmentBytes*4/3 + 64<<10
)

// ClientOptions is everything the client needs to talk to a running daemon.
type ClientOptions struct {
	// BaseAddress is where the daemon serves its HTTP interface, such as
	// "http://127.0.0.1:8420".
	BaseAddress string
	// Account is the phone number the daemon is linked to.
	Account string
	// Home is the agent's home folder, which is where attachments are cached.
	Home contract.Home
	// Secrets is the vault, whose redactor every outbound message goes through.
	Secrets contract.Secrets
}

// Client talks to one running signal-cli daemon over its HTTP interface: a
// health check, a remote-procedure endpoint for everything the agent does, and
// an event stream the reader in stream.go opens.
type Client struct {
	baseAddress string
	account     string
	secrets     contract.Secrets
	cache       *attachmentCache
	requests    *http.Client
	nextCallID  chan int
}

// NewClient builds a client for a daemon at an address. It talks to nothing
// until it is asked to.
func NewClient(options ClientOptions) (*Client, error) {
	if options.BaseAddress == "" {
		return nil, fmt.Errorf("the Signal client has no daemon address, so pass the address the daemon serves on")
	}
	if options.Secrets == nil {
		return nil, fmt.Errorf("the Signal client has no vault, and every message that leaves goes through the redactor first")
	}
	cache, err := newAttachmentCache(options.Home, AttachmentCacheLimit)
	if err != nil {
		return nil, err
	}
	client := &Client{
		baseAddress: strings.TrimRight(options.BaseAddress, "/"),
		account:     options.Account,
		secrets:     options.Secrets,
		cache:       cache,
		requests:    &http.Client{Timeout: callTimeout},
		nextCallID:  make(chan int, 1),
	}
	client.nextCallID <- 1
	return client, nil
}

// Health asks the daemon whether it is working, and says what is wrong when it
// is not.
func (client *Client) Health(ctx context.Context) contract.ChannelHealth {
	asked, stop := context.WithTimeout(ctx, healthTimeout)
	defer stop()

	request, err := http.NewRequestWithContext(asked, http.MethodGet, client.baseAddress+healthPath, nil)
	if err != nil {
		return contract.ChannelHealth{Detail: fmt.Sprintf("the daemon address %q cannot be used: %v", client.baseAddress, err)}
	}
	answer, err := client.requests.Do(request)
	if err != nil {
		return contract.ChannelHealth{Detail: fmt.Sprintf("signal-cli is not answering at %s, so start it or run \"coeus signal link\": %v", client.baseAddress, err)}
	}
	defer answer.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(answer.Body, maxAnswerBytes))

	if answer.StatusCode < 200 || answer.StatusCode >= 300 {
		return contract.ChannelHealth{Detail: fmt.Sprintf("signal-cli answered its health check with %s, so look at its log", answer.Status)}
	}
	return contract.ChannelHealth{Healthy: true}
}

// Send delivers one message, with any files, to one recipient. The text goes
// through the vault's redactor first, so that a secret can never leave this way.
func (client *Client) Send(ctx context.Context, recipient string, text string, files []string) error {
	if recipient == "" {
		return fmt.Errorf("cannot send a Signal message to nobody, so pass the recipient to send it to")
	}
	safe := client.secrets.Redact(text)
	if strings.TrimSpace(safe) == "" && len(files) == 0 {
		return fmt.Errorf("cannot send an empty Signal message, so pass some words or a file to send")
	}

	parameters := map[string]any{
		"recipient": []string{recipient},
		"message":   safe,
	}
	if len(files) > 0 {
		parameters["attachments"] = files
	}
	return client.call(ctx, "send", parameters, nil)
}

// Typing tells the recipient that a reply is being written, or that it is no
// longer being written. Signal drops the indicator on its own after about
// fifteen seconds, so this is repeated while a reply is being made.
func (client *Client) Typing(ctx context.Context, recipient string, stopping bool) error {
	if recipient == "" {
		return fmt.Errorf("cannot show a typing indicator to nobody, so pass the recipient it is for")
	}
	parameters := map[string]any{"recipient": []string{recipient}}
	if stopping {
		parameters["stop"] = true
	}
	return client.call(ctx, "sendTyping", parameters, nil)
}

// Download fetches one attachment from the daemon and keeps it in the cache,
// returning where it landed. The daemon hands the bytes back as base64 in its
// answer, because it holds the file itself and there is no other way to ask.
func (client *Client) Download(ctx context.Context, attachment Attachment, from string) (string, error) {
	if attachment.ID == "" {
		return "", fmt.Errorf("cannot download an attachment with no identifier, so pass the one the daemon reported")
	}
	if attachment.Size > maxAttachmentBytes {
		return "", fmt.Errorf("the attachment %s is %d bytes, which is more than the %d Signal allows, so it was left alone", attachment.ID, attachment.Size, maxAttachmentBytes)
	}

	parameters := map[string]any{"id": attachment.ID}
	if from != "" {
		parameters["recipient"] = from
	}
	var answer struct {
		Data string `json:"data"`
	}
	if err := client.call(ctx, "getAttachment", parameters, &answer); err != nil {
		return "", err
	}
	content, err := base64.StdEncoding.DecodeString(answer.Data)
	if err != nil || len(content) == 0 {
		return "", fmt.Errorf("the daemon handed back no readable bytes for the attachment %s, so it was left alone", attachment.ID)
	}
	return client.cache.save(attachment, content)
}

// call makes one remote-procedure call and reads the result into the value
// given, which may be nil when there is nothing to read.
func (client *Client) call(ctx context.Context, method string, parameters map[string]any, into any) error {
	if client.account != "" {
		parameters["account"] = client.account
	}
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  parameters,
		"id":      client.takeCallID(),
	})
	if err != nil {
		return fmt.Errorf("cannot write the %s call to signal-cli as JSON: %w", method, err)
	}

	answer, err := client.post(ctx, body, method)
	if err != nil {
		return err
	}
	if answer == nil {
		return nil
	}
	return readResult(answer, method, into)
}

// post sends one call and returns the answer, or nil when the daemon answered
// that it had done the work and had nothing to say about it.
func (client *Client) post(ctx context.Context, body []byte, method string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseAddress+remoteProcedurePath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("cannot build the %s call to signal-cli: %w", method, err)
	}
	request.Header.Set("Content-Type", "application/json")

	answer, err := client.requests.Do(request)
	if err != nil {
		return nil, fmt.Errorf("signal-cli did not answer the %s call, so check that it is still running: %w", method, err)
	}
	defer answer.Body.Close()

	written, err := io.ReadAll(io.LimitReader(answer.Body, maxAttachmentAnswerBytes))
	if err != nil {
		return nil, fmt.Errorf("cannot read what signal-cli answered the %s call with: %w", method, err)
	}
	if answer.StatusCode == http.StatusCreated {
		return nil, nil
	}
	if len(written) == 0 {
		return nil, fmt.Errorf("signal-cli answered the %s call with nothing at all and the status %s", method, answer.Status)
	}
	return written, nil
}

// takeCallID hands out the next number to label a call with, which is what the
// answer is matched to. One at a time is enough, because every call waits for
// its own answer.
func (client *Client) takeCallID() int {
	number := <-client.nextCallID
	client.nextCallID <- number + 1
	return number
}

// readResult reads one answer, turning what the daemon complained about into an
// error and everything else into the value given.
func readResult(answer []byte, method string, into any) error {
	var envelope struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(answer, &envelope); err != nil {
		return fmt.Errorf("signal-cli answered the %s call with something that is not JSON: %w", method, err)
	}
	if envelope.Error != nil {
		return fmt.Errorf("signal-cli refused the %s call with error %d: %s", method, envelope.Error.Code, envelope.Error.Message)
	}
	if into == nil || len(envelope.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(envelope.Result, into); err != nil {
		return fmt.Errorf("signal-cli answered the %s call in a shape this version cannot read: %w", method, err)
	}
	return nil
}
