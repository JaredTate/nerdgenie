//go:build integration

package command_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/command"
	"github.com/JaredTate/coeus/internal/config"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/log"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/vault"
)

func TestUndoAgainstTheRealEventLog(t *testing.T) {
	home := testkit.NewTempHome(t)
	store, err := log.Open(context.Background(), home.DatabaseFile())
	if err != nil {
		t.Fatalf("opening the real event log failed: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	folder := t.TempDir()
	notes := filepath.Join(folder, "notes.md")
	if err := os.WriteFile(notes, []byte("the words the agent wrote"), contract.DataFileMode); err != nil {
		t.Fatalf("writing the file to be undone failed: %v", err)
	}
	appendRealEvent(t, store, contract.EventMessage, map[string]string{"text": "write up the anniversary"})
	appendRealEvent(t, store, contract.EventFileChange, contract.FileChangeBody{
		Path: notes, Existed: true, PriorContents: []byte("the words the user wrote"), Mode: uint32(contract.DataFileMode),
	})

	answer := runOne(t, command.Deps{Store: store}, "/undo")

	restored, err := os.ReadFile(notes)
	if err != nil {
		t.Fatalf("reading the restored file failed: %v", err)
	}
	if string(restored) != "the words the user wrote" {
		t.Errorf("the file came back as %q rather than what it held before the turn", restored)
	}
	if !strings.Contains(answer, notes) {
		t.Errorf("the undo command does not say which file it put back: %q", answer)
	}
}

func TestInitOnTheRealFilesystemLeavesAHomeTheDoctorAndTheVaultAgreeWith(t *testing.T) {
	home := emptyHome(t)
	noProgramsOnThePath(t)
	t.Setenv("A_KEY_FOR_THE_INTEGRATION_TEST", "sk-the-key-itself")
	written := &strings.Builder{}

	err := command.Init(context.Background(), command.Setup{
		Home:            home,
		Output:          written,
		LocalAddress:    nothingListening(t),
		LMStudioAddress: nothingListening(t),
	}, []string{"--model", command.OpenAIAlias, "--api-key-from-env", "A_KEY_FOR_THE_INTEGRATION_TEST", "--yes"})
	if err != nil {
		t.Fatalf("coeus init failed: %v", err)
	}

	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("the configuration coeus init wrote will not load: %v", err)
	}
	if settings.DefaultModel != command.OpenAIAlias {
		t.Errorf("the configuration names %q as the model rather than the one the flag chose", settings.DefaultModel)
	}
	if report := config.Doctor(context.Background(), home); report.Verdict() == config.Trouble {
		t.Errorf("the doctor found something broken after coeus init:\n%s", report)
	}

	opened, err := vault.Open(home, testkit.NewFakeClock(theStartOfTime))
	if err != nil {
		t.Fatalf("opening the vault coeus init made failed: %v", err)
	}
	defer func() { _ = opened.Close() }()
	held, err := opened.Resolve(context.Background(), contract.SecretReferencePrefix+command.OpenAIAlias)
	if err != nil {
		t.Fatalf("the vault does not hold the key coeus init was given: %v", err)
	}
	if held.Password != "sk-the-key-itself" {
		t.Errorf("the vault holds a different key from the one the environment named")
	}

	for _, path := range []string{home.VaultFile(), home.VaultKeyFile()} {
		about, err := os.Stat(path)
		if err != nil {
			t.Errorf("%s was not made: %v", path, err)
			continue
		}
		if about.Mode().Perm() != contract.SecretFileMode {
			t.Errorf("%s has mode %04o rather than %04o, so another account could read it", path, about.Mode().Perm(), contract.SecretFileMode)
		}
	}
}

// appendRealEvent writes one event into the real event log.
func appendRealEvent(t *testing.T, store contract.Store, kind contract.EventKind, body any) {
	t.Helper()
	written, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("writing the event body failed: %v", err)
	}
	if _, err := store.Append(context.Background(), contract.Event{TaskID: "17", Kind: kind, Body: written}); err != nil {
		t.Fatalf("appending the %s event failed: %v", kind, err)
	}
}
