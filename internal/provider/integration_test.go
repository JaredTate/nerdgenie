//go:build integration

package provider_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/provider"
)

// This test needs the real filesystem and a real child process, which is what
// separates it from the unit tests: it proves that the scratch folder a
// command-line run needs is really made under the real home layout, with the
// modes that layout calls for, and is really gone afterwards.

func TestTheScratchFolderIsMadeUnderTheRealHomeAndRemovedAfterwards(t *testing.T) {
	record := installFakeProgram(t, contract.ClaudeProgram, claudeFixture, 0)
	model, options, _ := commandLineModelFor(t, contract.ClaudeProgram)

	if _, err := model.Send(context.Background(), requestWithEverything(), nil); err != nil {
		t.Fatalf("one call through the claude program failed: %v", err)
	}

	where := strings.TrimSpace(record.read("cwd.txt"))
	if _, err := os.Stat(where); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the scratch folder %s is still on disk after the run, and every run cleans up after itself", where)
	}
	root := filepath.Join(options.Home.RunFolder(), "cli")
	held, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("the folder %s the runs happen under cannot be read: %v", root, err)
	}
	if len(held) != 0 {
		t.Errorf("the folder %s still holds %d entries after the run", root, len(held))
	}
	about, err := os.Stat(root)
	if err != nil {
		t.Fatalf("the folder %s cannot be looked at: %v", root, err)
	}
	if about.Mode().Perm() != contract.HomeFolderMode.Perm() {
		t.Errorf("the folder %s has the mode %v, want %v, because everything under the home folder is the agent's alone",
			root, about.Mode().Perm(), contract.HomeFolderMode.Perm())
	}
}

func TestTwoRunsAtOnceGetFoldersOfTheirOwn(t *testing.T) {
	installFakeProgram(t, contract.ClaudeProgram, claudeFixture, 0)
	model, options, _ := commandLineModelFor(t, contract.ClaudeProgram)

	failures := make(chan error, 4)
	for range 4 {
		go func() {
			_, err := model.Send(context.Background(), requestWithEverything(), nil)
			failures <- err
		}()
	}
	for range 4 {
		if err := <-failures; err != nil {
			t.Errorf("one of four calls made at the same time failed: %v", err)
		}
	}

	root := filepath.Join(options.Home.RunFolder(), "cli")
	held, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("the folder %s the runs happen under cannot be read: %v", root, err)
	}
	if len(held) != 0 {
		t.Errorf("the folder %s still holds %d entries after four runs", root, len(held))
	}
}

func TestTheOpenAIProviderWorksOverARealLoopbackConnection(t *testing.T) {
	double := llamaServerSaying(65536)
	defer double.Close()
	options, recorder := testOptions(t, newTestClock())
	model, err := provider.New(localAliasAt(double.URL, 262144), options)
	if err != nil {
		t.Fatalf("building the local provider failed: %v", err)
	}

	reply, streamed, err := sendAndCollect(context.Background(), model, requestWithEverything())

	if err != nil {
		t.Fatalf("one call over a real loopback connection failed: %v", err)
	}
	if reply.Text != "ready" || streamed != reply.Text {
		t.Errorf("the reply is %q and the deltas joined to %q, want the server's word in both", reply.Text, streamed)
	}
	if model.ContextLength() != 65536 {
		t.Errorf("the model reports a window of %d, and the server said it loaded 65536", model.ContextLength())
	}
	if recorder.count() != 1 {
		t.Errorf("the shortened window was not written down exactly once: %v", recorder.all())
	}
}
