// The address of OpenAI's Codex backend and the headers it lets through were
// read from Hermes: the base address, and that the sign-in is the codex
// program's rather than a key, from
// ~/Code/hermes-agent/plugins/model-providers/openai-codex/__init__.py; the
// originator, the user agent shaped like the codex program's, and the account
// header from ~/Code/hermes-agent/agent/codex_headers.py, where they are set
// because the layer in front of the backend turns away a caller that does not
// send them; and the place the sign-in file lives, with the variable that
// moves it, from ~/Code/hermes-agent/hermes_cli/auth.py
// (_import_codex_cli_tokens). The installed copy under ~/.hermes/hermes-agent
// that the brief named says the same. The session_id header is the one the
// codex program itself sends on every call, so that the backend can scope its
// prompt cache to one conversation.
//
// Why this provider exists: the codex program, run as a bare model, describes
// its own tools to the backend in a developer item that nothing removes, so
// the model uses those and the harness's loop never drives it. Speaking the
// backend's wire directly puts the harness's own tools, and nothing else, in
// front of the model.

package provider

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The backend's address, the fixed headers it wants, and where the codex
// program keeps its sign-in.
const (
	// codexBackendAddress is where the Codex backend lives when the alias
	// names no address of its own.
	codexBackendAddress = "https://chatgpt.com/backend-api/codex"
	// codexOriginator is the caller name the layer in front of the backend
	// lets through, which is the codex program's own.
	codexOriginator = "codex_cli_rs"
	// codexUserAgent is shaped like the codex program's, for the same reason.
	codexUserAgent = "codex_cli_rs/0.0.0 (Nerd Genie)"
	// codexBetaHeader is the Responses API's beta flag the codex program sends.
	codexBetaHeader = "responses=experimental"
	// codexHomeVariable names the folder the codex program keeps its files in
	// when the user has moved it, which is also how a test keeps the real
	// sign-in out of reach.
	codexHomeVariable = "CODEX_HOME"
	// codexHomeFolder is the folder under the user's home directory the codex
	// program uses when the variable is unset.
	codexHomeFolder = ".codex"
	// codexLoginFileName is the file the codex program writes its sign-in to.
	codexLoginFileName = "auth.json"
)

// codexModel is one model reached through the Responses API on OpenAI's
// Codex backend, on the user's ChatGPT subscription, with the sign-in the
// codex program keeps.
type codexModel struct {
	alias   contract.ModelAlias
	options Options
	// sessionID names this model's conversation to the backend, which scopes
	// its prompt cache by it. One is made when the model is built.
	sessionID string
}

// newCodexModel returns the model a codex alias names. The sign-in file is
// read on every call rather than here, so that a token the codex program
// refreshed in the meantime is picked up, and so that a machine where codex is
// only a fallback still starts.
func newCodexModel(alias contract.ModelAlias, options Options) *codexModel {
	return &codexModel{alias: alias, options: options, sessionID: newSessionID()}
}

// newSessionID makes one random identifier in the shape the backend expects,
// which is the shape the codex program uses for its conversations.
func newSessionID() string {
	raw := make([]byte, 16)
	rand.Read(raw)
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16])
}

// Name is the alias the user gave this model.
func (model *codexModel) Name() string { return model.alias.Name }

// ContextLength is how many tokens the model holds on one call.
func (model *codexModel) ContextLength() int { return model.alias.ContextLength }

// Send makes one call and streams the reply back.
func (model *codexModel) Send(ctx context.Context, request contract.Request,
	onDelta func(delta string)) (contract.Reply, error) {
	request.Think = thinkFor(request, model.alias)
	if err := CheckThink(model.alias, request.Think); err != nil {
		return contract.Reply{}, err
	}
	body, err := codexRequestBody(request, model.alias.ModelName)
	if err != nil {
		return contract.Reply{}, err
	}
	login, err := readCodexLogin(codexLoginPath(), model.options.Clock.Now())
	if err != nil {
		return contract.Reply{}, err
	}

	stream, err := openStreamedCall(ctx, model.options, model.alias.Name, model.address(), model.headers(login), body)
	if err != nil {
		return contract.Reply{}, model.explainRefusal(err)
	}
	defer stream.finish()
	result, err := readCodexStream(stream.reader, onDelta)
	if err != nil {
		return contract.Reply{}, model.explainStreamFailure(stream, err)
	}
	return contract.Reply{
		Text:      result.Text,
		ToolCalls: result.ToolCalls,
		Finish:    result.Finish,
		Usage:     result.Usage,
		Model:     model.alias.Name,
	}, nil
}

// codexLoginPath is where the codex program's sign-in file lives: under the
// folder the variable names when it is set, and under the user's home
// directory otherwise.
func codexLoginPath() string {
	folder := strings.TrimSpace(os.Getenv(codexHomeVariable))
	if folder == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			userHome = "."
		}
		folder = filepath.Join(userHome, codexHomeFolder)
	}
	return filepath.Join(folder, codexLoginFileName)
}

// address is where the Responses API lives on this backend.
func (model *codexModel) address() string {
	base := model.alias.BaseAddress
	if base == "" {
		base = codexBackendAddress
	}
	return strings.TrimSuffix(base, "/") + "/responses"
}

// headers are what every call carries: the sign-in, and the headers the layer
// in front of the backend wants to see.
func (model *codexModel) headers(login codexLogin) http.Header {
	header := http.Header{}
	header.Set("Content-Type", "application/json")
	header.Set("Accept", "text/event-stream")
	header.Set("Authorization", "Bearer "+login.AccessToken)
	if login.AccountID != "" {
		header.Set("ChatGPT-Account-Id", login.AccountID)
	}
	header.Set("originator", codexOriginator)
	header.Set("User-Agent", codexUserAgent)
	header.Set("OpenAI-Beta", codexBetaHeader)
	header.Set("session_id", model.sessionID)
	return header
}

// explainRefusal adds the one thing the user can do to a refusal that says the
// sign-in was not accepted, because the backend's own words for it name
// nothing.
func (model *codexModel) explainRefusal(err error) error {
	failure := providerError{}
	if errors.As(err, &failure) && (failure.status == http.StatusUnauthorized || failure.status == http.StatusForbidden) {
		return fmt.Errorf("%w, which means the backend did not accept the sign-in, so %s", err, codexSignInAdvice)
	}
	return err
}

// explainStreamFailure names the model in a stream that could not be read to
// its end: the stalled-stream sentinel when the watch cut the call short, and
// otherwise a failure a later attempt could get past, because the backend
// giving up part way through is the backend's problem rather than the
// request's.
func (model *codexModel) explainStreamFailure(stream *openStream, err error) error {
	if stream.watch.stalled.Load() {
		return stream.watch.explain(model.alias.Name, err)
	}
	return providerError{modelName: model.alias.Name, message: err.Error(), retryable: true}
}
