package provider_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/provider"
)

// codexAnswer is one thing the fake Codex backend does when it is called: a
// refusal with a status and a body, a stream of server-sent events, or a stream
// that opens and then says nothing at all.
type codexAnswer struct {
	// status is the HTTP status to answer with, and zero means two hundred.
	status int
	// retryAfter is the value of the Retry-After header, or empty for none.
	retryAfter string
	// body is what a refusal says.
	body string
	// events is the whole server-sent-event stream a good answer sends.
	events string
	// stall says to open the stream and then send nothing until the caller
	// gives up, which is what a hung backend looks like.
	stall bool
}

// codexCall is one request the fake backend received, kept so that a test can
// look at what the provider put on the wire.
type codexCall struct {
	// path is the address the provider posted to.
	path string
	// header is every header the provider sent.
	header http.Header
	// body is the request body as it arrived.
	body []byte
}

// codexBackend is a fake of OpenAI's Codex backend: it keeps every request it
// was sent and answers each one with the next answer the test put in it, so
// that a test can drive a refusal, a retry, and a good stream in one call.
type codexBackend struct {
	server  *httptest.Server
	guard   sync.Mutex
	answers []codexAnswer
	calls   []codexCall
}

// newCodexBackend starts a fake backend that answers with the given answers in
// order, repeating the last one when it runs out.
func newCodexBackend(t *testing.T, answers ...codexAnswer) *codexBackend {
	t.Helper()
	backend := &codexBackend{answers: answers}
	backend.server = httptest.NewServer(http.HandlerFunc(backend.answer))
	t.Cleanup(backend.server.Close)
	return backend
}

// address is the base address a model alias points at this backend with.
func (backend *codexBackend) address() string { return backend.server.URL }

// answer keeps the request and writes back whatever the test scripted.
func (backend *codexBackend) answer(writer http.ResponseWriter, request *http.Request) {
	body, _ := readAllOf(request)
	backend.guard.Lock()
	at := len(backend.calls)
	backend.calls = append(backend.calls, codexCall{path: request.URL.Path, header: request.Header.Clone(), body: body})
	scripted := codexAnswer{}
	if len(backend.answers) > 0 {
		scripted = backend.answers[min(at, len(backend.answers)-1)]
	}
	backend.guard.Unlock()

	if scripted.retryAfter != "" {
		writer.Header().Set("Retry-After", scripted.retryAfter)
	}
	if scripted.status != 0 && scripted.status != http.StatusOK {
		writer.Header().Set("content-type", "application/json")
		writer.WriteHeader(scripted.status)
		fmt.Fprint(writer, scripted.body)
		return
	}
	writer.Header().Set("content-type", "text/event-stream")
	writer.WriteHeader(http.StatusOK)
	flusher, canFlush := writer.(http.Flusher)
	if canFlush {
		flusher.Flush()
	}
	if scripted.stall {
		<-request.Context().Done()
		return
	}
	fmt.Fprint(writer, scripted.events)
}

// readAllOf reads a request body whole, because every body a test sends is
// short enough to hold.
func readAllOf(request *http.Request) ([]byte, error) {
	defer request.Body.Close()
	body := strings.Builder{}
	buffer := make([]byte, 4096)
	for {
		read, err := request.Body.Read(buffer)
		body.Write(buffer[:read])
		if err != nil {
			return []byte(body.String()), nil
		}
	}
}

// lastCall is the last request the fake backend received.
func (backend *codexBackend) lastCall(t *testing.T) codexCall {
	t.Helper()
	backend.guard.Lock()
	defer backend.guard.Unlock()
	if len(backend.calls) == 0 {
		t.Fatal("the provider never called the fake Codex backend at all")
	}
	return backend.calls[len(backend.calls)-1]
}

// callCount is how many requests the fake backend received.
func (backend *codexBackend) callCount() int {
	backend.guard.Lock()
	defer backend.guard.Unlock()
	return len(backend.calls)
}

// lastBody is the body of the last request, read back as JSON.
func (backend *codexBackend) lastBody(t *testing.T) map[string]any {
	t.Helper()
	call := backend.lastCall(t)
	body := map[string]any{}
	if err := json.Unmarshal(call.body, &body); err != nil {
		t.Fatalf("the body the provider sent the Codex backend is not JSON: %v\n%s", err, call.body)
	}
	return body
}

// codexSecretToken is the access token every unit test writes into its fake
// sign-in file. It is deliberately unmistakable, so that a test can prove no
// error message and no log line ever carries it.
const codexSecretToken = "SECRET-CODEX-ACCESS-TOKEN-DO-NOT-PRINT"

// codexAccountFromFile is the account identifier the fake sign-in file names
// beside its token.
const codexAccountFromFile = "account-from-the-file"

// codexTokenExpiring returns a token shaped like the one the codex program
// keeps: three dot-separated parts, the middle one holding the claims, with the
// secret above in the signature so that a leak is easy to spot.
func codexTokenExpiring(at time.Time, accountInTheToken string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(
		`{"exp":%d,"https://api.openai.com/auth":{"chatgpt_account_id":%q}}`, at.Unix(), accountInTheToken)))
	return header + "." + claims + "." + codexSecretToken
}

// writeCodexLogin points the codex home at a folder of its own and writes the
// sign-in file the provider reads, so that no test ever touches the real one.
func writeCodexLogin(t *testing.T, contents string) string {
	t.Helper()
	folder := t.TempDir()
	t.Setenv("CODEX_HOME", folder)
	path := filepath.Join(folder, "auth.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("the fake sign-in file %s could not be written: %v", path, err)
	}
	return path
}

// goodCodexLogin is the sign-in file of a machine where the codex program is
// logged in and its token has not expired yet.
func goodCodexLogin(t *testing.T) {
	t.Helper()
	writeCodexLogin(t, codexLoginFile(codexTokenExpiring(testClockNow().Add(24*time.Hour), "account-from-the-token")))
}

// codexLoginFile is the shape of the codex program's auth.json around one
// token, with the account identifier beside it the way the real file has it.
func codexLoginFile(token string) string {
	return fmt.Sprintf(`{"auth_mode":"chatgpt","OPENAI_API_KEY":null,"tokens":{"id_token":"ignored","access_token":%q,"refresh_token":"ignored","account_id":%q},"last_refresh":"2026-09-02T00:00:00Z"}`,
		token, codexAccountFromFile)
}

// testClockNow is the moment the fake clock in this package reads, so that a
// token's expiry can be written on either side of it.
func testClockNow() time.Time {
	return time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
}

// codexAliasAt is a codex model alias pointing at the fake backend.
func codexAliasAt(address string, level contract.Think) contract.ModelAlias {
	return contract.ModelAlias{
		Name:          "gpt",
		Provider:      contract.ProviderCodex,
		BaseAddress:   address,
		ModelName:     "gpt-5.6-sol",
		ContextLength: 400000,
		Think:         level,
	}
}

// codexAgainst builds the Codex provider pointed at the fake backend, with a
// valid sign-in file already written.
func codexAgainst(t *testing.T, backend *codexBackend) (contract.Model, *noteRecorder) {
	t.Helper()
	goodCodexLogin(t)
	options, lines := testOptions(t, newTestClock())
	model, err := provider.New(codexAliasAt(backend.address(), contract.ThinkDefault), options)
	if err != nil {
		t.Fatalf("building the Codex provider failed: %v", err)
	}
	return model, lines
}

// codexEvents joins the lines of a scripted stream into the server-sent-event
// shape the backend really sends: an event name, a data line, and a blank line.
func codexEvents(payloads ...string) string {
	written := strings.Builder{}
	for _, payload := range payloads {
		name := struct {
			Type string `json:"type"`
		}{}
		_ = json.Unmarshal([]byte(payload), &name)
		written.WriteString("event: " + name.Type + "\ndata: " + payload + "\n\n")
	}
	return written.String()
}

// codexTextStream is the stream a plain text answer arrives as, with the words
// split into two deltas so that a test can prove they are joined.
func codexTextStream(first, second string) string {
	return codexEvents(
		`{"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}`,
		`{"type":"response.output_item.added","item":{"id":"msg_1","type":"message","status":"in_progress","content":[],"phase":"final_answer","role":"assistant"}}`,
		fmt.Sprintf(`{"type":"response.output_text.delta","delta":%q,"item_id":"msg_1"}`, first),
		fmt.Sprintf(`{"type":"response.output_text.delta","delta":%q,"item_id":"msg_1"}`, second),
		fmt.Sprintf(`{"type":"response.output_item.done","item":{"id":"msg_1","type":"message","status":"completed","phase":"final_answer","role":"assistant","content":[{"type":"output_text","text":%q}]}}`, first+second),
		`{"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":6100,"input_tokens_details":{"cached_tokens":5200},"output_tokens":400,"total_tokens":6500}}}`,
	)
}

// codexTwoCallStream is the stream a reply asking for two tools arrives as,
// with each call's arguments sent as a delta before the finished item.
func codexTwoCallStream() string {
	return codexEvents(
		`{"type":"response.created","response":{"id":"resp_2","status":"in_progress"}}`,
		`{"type":"response.output_item.added","item":{"id":"fc_1","type":"function_call","status":"in_progress","arguments":"","call_id":"call_1","name":"read"}}`,
		`{"type":"response.function_call_arguments.delta","delta":"{\"path\":\"notes.md\"}","item_id":"fc_1"}`,
		`{"type":"response.output_item.done","item":{"id":"fc_1","type":"function_call","status":"completed","arguments":"{\"path\":\"notes.md\"}","call_id":"call_1","name":"read"}}`,
		`{"type":"response.output_item.added","item":{"id":"fc_2","type":"function_call","status":"in_progress","arguments":"","call_id":"call_2","name":"write"}}`,
		`{"type":"response.output_item.done","item":{"id":"fc_2","type":"function_call","status":"completed","arguments":"{\"path\":\"draft.md\",\"text\":\"hello\"}","call_id":"call_2","name":"write"}}`,
		`{"type":"response.completed","response":{"id":"resp_2","status":"completed","usage":{"input_tokens":6100,"input_tokens_details":{"cached_tokens":5200},"output_tokens":400,"total_tokens":6500}}}`,
	)
}
