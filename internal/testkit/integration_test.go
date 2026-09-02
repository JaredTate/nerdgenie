//go:build integration

// This file is the wave-0 integration test: it puts several packages together
// through their real interfaces, on the real filesystem in a temporary home and
// over a real local socket, rather than in memory. It runs under the integration
// build tag, which is what "make test" and "make check" pass.
package testkit_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestTheTemporaryHomeIsRealOnDiskAndHoldsWhatTheFakesWrite(t *testing.T) {
	ctx := context.Background()
	home := testkit.NewTempHome(t)

	// A skill folder written through the contract lands where the layout says.
	skills := testkit.NewFakeSkill()
	if err := skills.Save(ctx, "post-to-x", map[string][]byte{
		"SKILL.md": []byte("# post-to-x\nPosts one message to X.\n"),
	}); err != nil {
		t.Fatalf("saving a skill failed: %v", err)
	}
	// The skills folder itself is one NewTempHome made, so only the skill's own
	// folder is made here. A temporary home that made nothing would fail on the
	// next line rather than being papered over.
	if _, err := os.Stat(home.SkillsFolder()); err != nil {
		t.Fatalf("the temporary home has no skills folder, so it did not build the layout: %v", err)
	}
	folder := home.SkillFolder("post-to-x")
	if err := os.Mkdir(folder, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the skill folder on disk: %v", err)
	}
	for name, content := range skills.Files("post-to-x") {
		path := filepath.Join(folder, name)
		if err := os.WriteFile(path, content, contract.DataFileMode); err != nil {
			t.Fatalf("cannot write %s: %v", path, err)
		}
	}

	written, err := os.ReadFile(filepath.Join(folder, "SKILL.md"))
	if err != nil {
		t.Fatalf("cannot read the skill back off the disk: %v", err)
	}
	if !strings.Contains(string(written), "post-to-x") {
		t.Errorf("the skill on disk reads %q, want the one that was saved", written)
	}

	// The vault file, when it exists, must be readable by nobody else.
	if err := os.WriteFile(home.VaultFile(), []byte("not a real vault"), contract.SecretFileMode); err != nil {
		t.Fatalf("cannot write the vault file: %v", err)
	}
	info, err := os.Stat(home.VaultFile())
	if err != nil {
		t.Fatalf("cannot read the vault file's mode: %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Errorf("the vault file has mode %#o, which lets somebody other than the agent's user read it", info.Mode().Perm())
	}
}

func TestTheGoldenHelperWritesAndComparesARealFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "record.txt")
	wanted := []byte("# task 17   running   from Signal\n")

	if err := testkit.WriteGolden(path, wanted); err != nil {
		t.Fatalf("writing the golden file failed: %v", err)
	}
	same, err := testkit.GoldenMatches(path, wanted)
	if err != nil || !same {
		t.Fatalf("the golden file did not match what was just written: same=%v err=%v", same, err)
	}
	if same, _ := testkit.GoldenMatches(path, []byte("something else")); same {
		t.Error("the golden file matched bytes that differ from it")
	}
}

func TestTheFortyStepFixtureLoadsFromTheRealRepositoryAndPassesItsThreeAssertions(t *testing.T) {
	task, err := testkit.LoadFortyStepTask()
	if err != nil {
		t.Fatalf("loading the fixture from the repository failed: %v", err)
	}

	record := task.RecordAtTheEnd()
	if err := task.CheckAskAndCorrections(record); err != nil {
		t.Errorf("the ask or a correction changed: %v", err)
	}
	if err := task.CheckDoneList(record); err != nil {
		t.Errorf("the done-check failed: %v", err)
	}

	held := map[string]string{}
	for _, result := range task.ToolResults() {
		held[result.ID] = result.Text
	}
	err = task.CheckResultsReadable(func(id string) (string, error) {
		text, found := held[id]
		if !found {
			return "", errNoSuchResult
		}
		return text, nil
	})
	if err != nil {
		t.Errorf("a result is no longer readable by its id: %v", err)
	}
}

func TestTheBrowserWorkerSpeaksItsProtocolOverARealSocket(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	server := testkit.NewBrowserProtocolServer(t, worker)

	if _, err := os.Stat(server.SocketPath()); err != nil {
		t.Fatalf("the socket is not on the filesystem: %v", err)
	}

	answer := callProtocol(t, server.SocketPath(),
		`{"jsonrpc":"2.0","id":1,"method":"open","params":{"url":"`+testkit.FixtureSimplePage+`"}}`)
	result, isResult := answer["result"].(map[string]any)
	if !isResult || result["url"] != testkit.FixtureSimplePage {
		t.Fatalf("the answer over the real socket is %+v, want the simple fixture page", answer)
	}
}

func TestEveryFakeKeepsItsContractInOnePass(t *testing.T) {
	ctx := context.Background()
	browser := testkit.NewFakeBrowserWorker()
	defer browser.Close()

	checks := []struct {
		name string
		run  func() error
	}{
		{"the clock", func() error { return testkit.CheckClock(testkit.NewFakeClock(time.Unix(0, 0).UTC())) }},
		{"the store", func() error { return testkit.CheckStore(ctx, testkit.NewFakeStore()) }},
		{"memory", func() error { return testkit.CheckMemory(ctx, testkit.NewFakeMemory()) }},
		{"the sandbox", func() error { return testkit.CheckSandbox(ctx, testkit.NewFakeSandbox()) }},
		{"the vault", func() error { return testkit.CheckSecrets(ctx, testkit.NewFakeSecrets()) }},
		{"permissions", func() error {
			return testkit.CheckPermission(ctx, testkit.NewFakePermission(contract.RulingAllow))
		}},
		{"skills", func() error { return testkit.CheckSkill(ctx, testkit.NewFakeSkill()) }},
		{"jobs", func() error {
			return testkit.CheckJob(ctx, testkit.NewFakeJob(testkit.NewFakeClock(time.Unix(0, 0).UTC())))
		}},
		{"the channel", func() error { return testkit.CheckChannel(ctx, testkit.NewFakeChannel("terminal")) }},
		{"the browser worker", func() error { return testkit.CheckBrowserWorker(ctx, browser) }},
		{"the desktop", func() error { return testkit.CheckDesktop(ctx, testkit.NewFakeDesktop()) }},
	}
	for _, check := range checks {
		if err := check.run(); err != nil {
			t.Errorf("%s does not keep its contract: %v", check.name, err)
		}
	}
}
