package write_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/tool/write"
)

func TestAFolderNamedWhereAFileShouldBeIsRefused(t *testing.T) {
	tool, root, _ := newTool(t)
	folder := filepath.Join(root, "drafts")
	if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the folder: %v", err)
	}

	_, err := run(t, tool, map[string]any{"path": folder, "content": "no"})
	if err == nil {
		t.Fatalf("a folder was written over as though it were a file")
	}
	if !strings.Contains(err.Error(), "folder") {
		t.Errorf("the refusal reads %q and does not say the path is a folder", err)
	}
}

func TestAFileTooBigToKeepIsChangedWithNothingKeptOfWhatItHeld(t *testing.T) {
	tool, root, store := newTool(t)
	path := filepath.Join(root, "enormous.txt")
	if err := os.WriteFile(path, make([]byte, write.MaxPriorContentsBytes+1), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the enormous file: %v", err)
	}

	if _, err := run(t, tool, map[string]any{"path": path, "content": "small now"}); err != nil {
		t.Fatalf("overwriting an enormous file failed: %v", err)
	}
	changes := fileChanges(t, store)
	if len(changes) != 1 || !changes[0].Existed {
		t.Fatalf("the log holds %v, want one change saying the file was there", changes)
	}
	if len(changes[0].PriorContents) != 0 {
		t.Errorf("the log kept %d bytes of a file over the cap of %d", len(changes[0].PriorContents), write.MaxPriorContentsBytes)
	}
}

func TestAToolWithNoLogRefusesToWriteAnything(t *testing.T) {
	root := t.TempDir()
	tool := write.New(write.Settings{
		Allowed: func(asked string) (string, error) { return asked, nil },
		TaskID:  "17",
		Clock:   testkit.NewFakeClock(time.Unix(1700000000, 0).UTC()),
	})

	_, err := run(t, tool, map[string]any{"path": filepath.Join(root, "x.md"), "content": "no"})
	if err == nil {
		t.Fatalf("a file was written by a tool with no log to record the change in")
	}
	if !strings.Contains(err.Error(), "log") {
		t.Errorf("the refusal reads %q and does not say what is missing", err)
	}
}

func TestAToolWithNoFoldersToWriteInSaysSo(t *testing.T) {
	tool := write.New(write.Settings{})

	_, err := run(t, tool, map[string]any{"path": "/tmp/anything", "content": "no"})
	if err == nil {
		t.Fatalf("a file was written by a tool with no folders wired into it")
	}
	if !strings.Contains(err.Error(), "sandbox roots") {
		t.Errorf("the refusal reads %q and does not say what is missing", err)
	}
}

func TestAFileTheAgentMayNotReadIsRefusedRatherThanOverwritten(t *testing.T) {
	tool, root, _ := newTool(t)
	shut := filepath.Join(root, "shut")
	if err := os.MkdirAll(shut, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the folder: %v", err)
	}
	path := filepath.Join(shut, "held.txt")
	if err := os.WriteFile(path, []byte("held\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the file: %v", err)
	}
	if err := os.Chmod(shut, 0o600); err != nil {
		t.Fatalf("cannot shut the folder: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(shut, contract.HomeFolderMode) })

	if _, err := run(t, tool, map[string]any{"path": path, "content": "no"}); err == nil {
		t.Errorf("a file the agent cannot look at was written over")
	}
}

func TestTheTimeOnTheChangeComesFromTheClock(t *testing.T) {
	root := t.TempDir()
	store := testkit.NewFakeStore()
	moment := time.Unix(1700000000, 0).UTC()
	tool := write.New(write.Settings{
		Allowed: func(asked string) (string, error) { return asked, nil },
		Log:     store,
		TaskID:  "17",
		Clock:   testkit.NewFakeClock(moment),
	})

	if _, err := run(t, tool, map[string]any{"path": filepath.Join(root, "x.md"), "content": "yes"}); err != nil {
		t.Fatalf("writing a file failed: %v", err)
	}
	events, err := store.ByKind(context.Background(), contract.EventFileChange)
	if err != nil || len(events) != 1 {
		t.Fatalf("the log holds %d file-change events: %v", len(events), err)
	}
	if !events[0].Occurred.Equal(moment) {
		t.Errorf("the change happened at %s, want the time the clock reads, %s", events[0].Occurred, moment)
	}
}
