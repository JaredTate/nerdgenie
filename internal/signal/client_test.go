package signal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// errStubRefused is what the stub daemon answers with when a test wants to see
// a call refused.
var errStubRefused = errors.New("the daemon is unhappy about that call")

// newTestClient builds a client pointed at an address, on a temporary home, with
// an empty vault the test can add secrets to.
func newTestClient(t *testing.T, baseAddress string) (*Client, *testkit.FakeSecrets) {
	t.Helper()
	secrets := testkit.NewFakeSecrets()
	client, err := NewClient(ClientOptions{
		BaseAddress: baseAddress,
		Account:     "+15125550100",
		Home:        testkit.NewTempHome(t),
		Secrets:     secrets,
	})
	if err != nil {
		t.Fatalf("cannot build the signal-cli client: %v", err)
	}
	return client, secrets
}

// newRPCStub serves the daemon's three paths, answering the remote-procedure
// calls with the function given. The fake signal-cli in testkit answers every
// call with a timestamp and nothing else, which is right for a send and wrong
// for a call that has to return something, so the calls that return something
// are tested here.
func newRPCStub(t *testing.T, answer func(method string, params map[string]any) (any, error)) string {
	t.Helper()
	router := http.NewServeMux()
	router.HandleFunc(healthPath, func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, map[string]any{"status": "ok"})
	})
	router.HandleFunc(remoteProcedurePath, func(writer http.ResponseWriter, request *http.Request) {
		var call struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
			http.Error(writer, "not JSON", http.StatusBadRequest)
			return
		}
		result, err := answer(call.Method, call.Params)
		if err != nil {
			writeJSON(writer, map[string]any{"error": map[string]any{"code": -1, "message": err.Error()}})
			return
		}
		writeJSON(writer, map[string]any{"jsonrpc": "2.0", "id": "1", "result": result})
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server.URL
}

// writeJSON writes one JSON body, which is all the stub ever answers with.
func writeJSON(writer http.ResponseWriter, body any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(body)
}

func TestClientHealthFollowsTheDaemon(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	client, _ := newTestClient(t, strings.TrimSuffix(daemon.HealthAddress(), testkit.SignalHealthPath))

	health := client.Health(context.Background())
	if !health.Healthy {
		t.Errorf("the client says the running daemon is not healthy: %s", health.Detail)
	}

	daemon.Close()
	health = client.Health(context.Background())
	if health.Healthy {
		t.Errorf("the client says a daemon that is gone is healthy")
	}
	if health.Detail == "" {
		t.Errorf("the client says the daemon is not healthy and does not say why")
	}
}

func TestClientSendsAMessageWithATypingIndicatorAndAFile(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	client, _ := newTestClient(t, strings.TrimSuffix(daemon.HealthAddress(), testkit.SignalHealthPath))

	if err := client.Typing(context.Background(), "+15125550123", false); err != nil {
		t.Fatalf("sending a typing indicator failed: %v", err)
	}
	if err := client.Send(context.Background(), "+15125550123", "here it is", []string{"/tmp/shot.png"}); err != nil {
		t.Fatalf("sending a message failed: %v", err)
	}

	if daemon.TypingIndicators() != 1 {
		t.Errorf("the daemon saw %d typing indicators, want one", daemon.TypingIndicators())
	}
	sends := daemon.Sends()
	if len(sends) != 1 {
		t.Fatalf("the daemon saw %d sends, want one", len(sends))
	}
	if len(sends[0].Recipients) != 1 || sends[0].Recipients[0] != "+15125550123" {
		t.Errorf("the message went to %v, want the one recipient", sends[0].Recipients)
	}
	if sends[0].Message != "here it is" {
		t.Errorf("the message says %q, want what was sent", sends[0].Message)
	}
	if len(sends[0].Attachments) != 1 || sends[0].Attachments[0] != "/tmp/shot.png" {
		t.Errorf("the message carried %v, want the one file", sends[0].Attachments)
	}
}

func TestClientBlacksOutASecretBeforeItLeaves(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	client, secrets := newTestClient(t, strings.TrimSuffix(daemon.HealthAddress(), testkit.SignalHealthPath))
	secrets.Add("x-account", contract.Credential{Site: "x-account", Password: "hunter2seventeen"})

	if err := client.Send(context.Background(), "+15125550123", "the password is hunter2seventeen", nil); err != nil {
		t.Fatalf("sending failed: %v", err)
	}

	sends := daemon.Sends()
	if len(sends) != 1 {
		t.Fatalf("the daemon saw %d sends, want one", len(sends))
	}
	if strings.Contains(sends[0].Message, "hunter2seventeen") {
		t.Errorf("the message %q carries a secret out of the program", sends[0].Message)
	}
	if !strings.Contains(sends[0].Message, contract.RedactedMarker) {
		t.Errorf("the message %q does not show where the secret was taken out", sends[0].Message)
	}
}

func TestClientRefusesAMessageWithNothingInIt(t *testing.T) {
	daemon := testkit.NewFakeSignalCLI()
	defer daemon.Close()
	client, _ := newTestClient(t, strings.TrimSuffix(daemon.HealthAddress(), testkit.SignalHealthPath))

	if err := client.Send(context.Background(), "+15125550123", "   ", nil); err == nil {
		t.Errorf("the client sent an empty message, which signal-cli refuses anyway")
	}
	if err := client.Send(context.Background(), "", "hello", nil); err == nil {
		t.Errorf("the client sent a message to nobody")
	}
}

func TestClientDownloadsAnAttachmentIntoTheCache(t *testing.T) {
	wanted := []byte("these are the bytes of the photo")
	address := newRPCStub(t, func(method string, params map[string]any) (any, error) {
		if method != "getAttachment" {
			return map[string]any{"timestamp": 1}, nil
		}
		if params["id"] != "abc123" {
			t.Errorf("the daemon was asked for attachment %v, want abc123", params["id"])
		}
		if params["recipient"] != "+15125550123" {
			t.Errorf("the daemon was asked with recipient %v, want the sender the file came from", params["recipient"])
		}
		return map[string]any{"data": base64.StdEncoding.EncodeToString(wanted)}, nil
	})
	client, _ := newTestClient(t, address)

	path, err := client.Download(context.Background(), Attachment{ID: "abc123", Filename: "photo.jpg"}, "+15125550123")
	if err != nil {
		t.Fatalf("downloading an attachment failed: %v", err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the downloaded attachment: %v", err)
	}
	if string(written) != string(wanted) {
		t.Errorf("the downloaded attachment holds %q, want the bytes the daemon sent", written)
	}
}

func TestClientSaysWhatWentWrongWhenTheDaemonComplains(t *testing.T) {
	address := newRPCStub(t, func(_ string, _ map[string]any) (any, error) {
		return nil, errStubRefused
	})
	client, _ := newTestClient(t, address)

	err := client.Send(context.Background(), "+15125550123", "hello", nil)
	if err == nil {
		t.Fatalf("the client did not report a daemon that refused the call")
	}
	if !strings.Contains(err.Error(), "the daemon is unhappy") {
		t.Errorf("the error is %q, want it to carry what the daemon said", err)
	}
}

func TestClientRefusesAnAttachmentTheDaemonWillNotHandOver(t *testing.T) {
	address := newRPCStub(t, func(_ string, _ map[string]any) (any, error) {
		return map[string]any{"timestamp": 1}, nil
	})
	client, _ := newTestClient(t, address)

	if _, err := client.Download(context.Background(), Attachment{ID: "abc123"}, "+15125550123"); err == nil {
		t.Errorf("the client made a file out of an answer that carried no bytes")
	}
}
