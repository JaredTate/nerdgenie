package contract_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

func TestACredentialNeverPrintsItsValues(t *testing.T) {
	credential := contract.Credential{Site: "x-account", Domains: []string{"x.com"}, Username: "jared", Password: "hunter2hunter2", TOTPSecret: "JBSWY3DPEHPK3PXP"}
	for name, printed := range map[string]string{
		"String":     credential.String(),
		"%v":         fmt.Sprintf("%v", credential),
		"%+v":        fmt.Sprintf("%+v", credential),
		"Sprint":     fmt.Sprint(credential),
		"pointer %v": fmt.Sprintf("%v", &credential),
	} {
		if printed != contract.SecretMarker {
			t.Errorf("printing a credential with %s gave %q, want %q", name, printed, contract.SecretMarker)
		}
	}
	encoded, err := json.Marshal(struct {
		Login contract.Credential `json:"login"`
	}{credential})
	if err != nil {
		t.Fatalf("encoding a credential failed: %v", err)
	}
	if strings.Contains(string(encoded), "hunter2") || !strings.Contains(string(encoded), contract.SecretMarker) {
		t.Errorf("a credential inside JSON came out as %s, want only the marker %q", encoded, contract.SecretMarker)
	}
}

func TestTheTerminalChannelHasOneName(t *testing.T) {
	if contract.TerminalChannelName != "terminal" {
		t.Errorf("the terminal channel's name is %q, want \"terminal\"", contract.TerminalChannelName)
	}
}
