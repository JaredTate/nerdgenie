package provider_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/provider"
	"github.com/JaredTate/coeus/internal/testkit"
)

// llamaServerDouble answers the props question the way llama-server does, keeps
// the body of every chat request, and answers one short stream. The fake
// provider server in testkit serves only the two API paths, so the props probe
// needs a server of its own.
type llamaServerDouble struct {
	*httptest.Server
	guard    sync.Mutex
	lastBody string
}

// llamaServerSaying starts a double that reports the window given.
func llamaServerSaying(contextLength int) *llamaServerDouble {
	double := &llamaServerDouble{}
	handler := http.NewServeMux()
	handler.HandleFunc("/props", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(writer, `{"default_generation_settings":{"params":{"temperature":0.7},"n_ctx":%d},"model_alias":"local-coder"}`, contextLength)
	})
	handler.HandleFunc(testkit.OpenAIPath, func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		double.remember(string(body))
		writer.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(writer, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ready\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(writer, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(writer, "data: [DONE]\n\n")
	})
	double.Server = httptest.NewServer(handler)
	return double
}

// remember keeps the body of the last chat request.
func (double *llamaServerDouble) remember(body string) {
	double.guard.Lock()
	defer double.guard.Unlock()
	double.lastBody = body
}

// body is the last chat request the double received.
func (double *llamaServerDouble) body() string {
	double.guard.Lock()
	defer double.guard.Unlock()
	return double.lastBody
}

// localAliasAt is a local model alias pointed at an address, with the window
// config.toml would carry.
func localAliasAt(address string, contextLength int) contract.ModelAlias {
	return contract.ModelAlias{
		Name:          contract.LocalModelAlias,
		Provider:      contract.ProviderOpenAI,
		BaseAddress:   address + "/v1",
		ModelName:     "local-coder",
		ContextLength: contextLength,
	}
}

func TestTheSmallerWindowTheLocalServerReportsWins(t *testing.T) {
	double := llamaServerSaying(32768)
	defer double.Close()
	options, recorder := testOptions(t, newTestClock())

	model, err := provider.New(localAliasAt(double.URL, 262144), options)

	if err != nil {
		t.Fatalf("building the local provider failed: %v", err)
	}
	if model.ContextLength() != 32768 {
		t.Errorf("the model reports a window of %d, and the server said it loaded 32768", model.ContextLength())
	}
	if recorder.count() != 1 || !strings.Contains(recorder.all()[0], "32768") {
		t.Errorf("the shortened window was not written down in one line: %v", recorder.all())
	}
}

func TestTheConfiguredWindowStandsWhenTheServerReportsALargerOne(t *testing.T) {
	double := llamaServerSaying(262144)
	defer double.Close()
	options, recorder := testOptions(t, newTestClock())

	model, err := provider.New(localAliasAt(double.URL, 65536), options)

	if err != nil {
		t.Fatalf("building the local provider failed: %v", err)
	}
	if model.ContextLength() != 65536 {
		t.Errorf("the model reports a window of %d, and only a smaller reported window replaces the configured one", model.ContextLength())
	}
	if recorder.count() != 0 {
		t.Errorf("nothing was shortened, so nothing should have been written down: %v", recorder.all())
	}
}

func TestTheThinkingOffHintGoesOnlyToAServerThatAnsweredTheProbe(t *testing.T) {
	double := llamaServerSaying(262144)
	defer double.Close()
	options, _ := testOptions(t, newTestClock())
	model, err := provider.New(localAliasAt(double.URL, 262144), options)
	if err != nil {
		t.Fatalf("building the local provider failed: %v", err)
	}

	if _, err := model.Send(context.Background(), requestWithEverything(), nil); err != nil {
		t.Fatalf("one call to the local provider failed: %v", err)
	}

	if !strings.Contains(double.body(), `"chat_template_kwargs":{"enable_thinking":false}`) {
		t.Errorf("the request to a llama-server carries no thinking-off hint:\n%s", double.body())
	}
}

func TestNoExtraFieldGoesToAServerThatDidNotAnswerTheProbe(t *testing.T) {
	server := testkit.NewFakeProviderServer(scriptSayingOneThing("ready"))
	defer server.Close()
	model, _ := openAIAgainst(t, server)

	if _, err := model.Send(context.Background(), requestWithEverything(), nil); err != nil {
		t.Fatalf("one call to the OpenAI-compatible provider failed: %v", err)
	}

	if model.ContextLength() != 262144 {
		t.Errorf("the model reports a window of %d, and the configured one stands when nothing answered", model.ContextLength())
	}
	body := string(server.Requests()[len(server.Requests())-1].Body)
	if strings.Contains(body, "chat_template_kwargs") {
		t.Errorf("a server that did not answer the probe was sent the thinking-off hint anyway:\n%s", body)
	}
}

func TestAServerThatIsNotOnThisMachineIsNeverProbed(t *testing.T) {
	options, recorder := testOptions(t, newTestClock())

	model, err := provider.New(localAliasAt("http://198.51.100.7:19091", 4096), options)

	if err != nil {
		t.Fatalf("building the provider for a remote address failed: %v", err)
	}
	if model.ContextLength() != 4096 {
		t.Errorf("the model reports a window of %d, want the configured 4096", model.ContextLength())
	}
	if recorder.count() != 0 {
		t.Errorf("a remote address was probed, and only a loopback address is: %v", recorder.all())
	}
}

func TestAnOpenAIAliasWithNoAddressIsRefused(t *testing.T) {
	options, _ := testOptions(t, newTestClock())

	_, err := provider.New(contract.ModelAlias{
		Name:          contract.LocalModelAlias,
		Provider:      contract.ProviderOpenAI,
		ModelName:     "local-coder",
		ContextLength: 4096,
	}, options)

	if err == nil {
		t.Fatal("an OpenAI-compatible alias with no base address was accepted, and there is nowhere to send the call")
	}
}
