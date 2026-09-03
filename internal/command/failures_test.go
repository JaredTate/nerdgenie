package command_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/command"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// errBrokenJobStore is what a store that cannot be reached answers with.
var errBrokenJobStore = errors.New("the job store could not be read, so check that the database is readable")

// brokenJobs is a job store whose listing and pausing both fail. Only those two
// are ever called, so the rest of the contract is left to the embedded value.
type brokenJobs struct {
	contract.Job
	listFails bool
}

// List fails when the test asked it to, and otherwise gives back one running job.
func (jobs brokenJobs) List(_ context.Context) ([]contract.JobSummary, error) {
	if jobs.listFails {
		return nil, errBrokenJobStore
	}
	return []contract.JobSummary{{ID: "4", Title: "run the anniversary campaign", State: contract.JobRunning, TasksTotal: 2}}, nil
}

// Pause always fails, which is how a test sees what "/pause" says when a job
// will not stop.
func (jobs brokenJobs) Pause(_ context.Context, _ string) error { return errBrokenJobStore }

// answerOf runs one line and gives back the answer, whether the command
// answered or failed, so that a table can check either.
func answerOf(t *testing.T, deps command.Deps, line string) (string, error) {
	t.Helper()
	registry := command.NewRegistry()
	for _, one := range command.New(registry, deps).All() {
		if err := registry.Register(one); err != nil {
			t.Fatalf("registering %s failed: %v", one.Name, err)
		}
	}
	return registry.Run(context.Background(), line, contract.CommandContext{Channel: terminalChannel()})
}

func TestModelWithNothingConfiguredSaysSoRatherThanGuessing(t *testing.T) {
	answer, err := answerOf(t, command.Deps{Settings: contract.Config{}}, "/model")
	if err != nil {
		t.Fatalf("the model command failed with an empty configuration: %v", err)
	}
	if !strings.Contains(answer, "not chosen yet") || !strings.Contains(answer, "nothing") {
		t.Errorf("the model command does not say that there is nothing configured: %q", answer)
	}
}

func TestModelWithOneAliasNamesJustThatOne(t *testing.T) {
	answer, err := answerOf(t, command.Deps{Settings: contract.DefaultConfig()}, "/model")
	if err != nil {
		t.Fatalf("the model command failed: %v", err)
	}
	if strings.Contains(answer, " and ") {
		t.Errorf("the model command lists one alias as though there were several: %q", answer)
	}
}

func TestModelSaysWhatTheChangeFailedWith(t *testing.T) {
	broken := errors.New("the model could not be switched while a task is running")
	_, err := answerOf(t, command.Deps{
		Settings: contract.DefaultConfig(),
		SetModel: func(string) error { return broken },
	}, "/model local")
	if !errors.Is(err, broken) {
		t.Errorf("the model command hid what the change failed with: %v", err)
	}
}

func TestStatusSaysWhatItCouldNotReach(t *testing.T) {
	answer, err := answerOf(t, command.Deps{
		Settings:        contract.DefaultConfig(),
		Jobs:            brokenJobs{listFails: true},
		CostSoFar:       func() contract.CostLine { return contract.CostLine{InputTokens: 10, OutputTokens: 2} },
		PendingPreviews: func(context.Context) ([]contract.Preview, error) { return nil, errBrokenJobStore },
		Channels:        func() []contract.Channel { return nil },
	}, "/status")
	if err != nil {
		t.Fatalf("the status command failed rather than reporting what it could not reach: %v", err)
	}
	for _, wanted := range []string{"could not be listed", "channels: none", "10 tokens in"} {
		if !strings.Contains(answer, wanted) {
			t.Errorf("the status report leaves out %q:\n%s", wanted, answer)
		}
	}
}

func TestPauseSaysWhichJobWouldNotStop(t *testing.T) {
	_, err := answerOf(t, command.Deps{Jobs: brokenJobs{}}, "/pause")
	if !errors.Is(err, errBrokenJobStore) {
		t.Errorf("the pause command hid what the job store failed with: %v", err)
	}
}

func TestPauseSaysWhenTheJobsCouldNotBeListed(t *testing.T) {
	_, err := answerOf(t, command.Deps{Jobs: brokenJobs{listFails: true}}, "/pause")
	if !errors.Is(err, errBrokenJobStore) {
		t.Errorf("the pause command hid what the listing failed with: %v", err)
	}
}

func TestPauseAndResumeCountOneJobAsOne(t *testing.T) {
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(theStartOfTime))
	if _, err := jobs.Create(context.Background(), contract.NewJob{Ask: "post the weekly update", Why: "because the user asked"}); err != nil {
		t.Fatalf("creating the job failed: %v", err)
	}
	resumed := []string{}

	paused, started := runTwo(t, jobDeps(jobs, &resumed), "/pause", "/resume")

	if !strings.Contains(paused, "one job") {
		t.Errorf("the pause command counts one job as something else: %q", paused)
	}
	if !strings.Contains(started, "one job") {
		t.Errorf("the resume command counts one job as something else: %q", started)
	}
}

// oneRunningJobPaused registers the core commands over a job store holding one
// running job, pauses it, and gives back the registry, so that a test can see
// what "/resume" does when starting it again goes wrong.
func oneRunningJobPaused(t *testing.T, deps command.Deps, jobs *testkit.FakeJob) *command.Registry {
	t.Helper()
	if _, err := jobs.Create(context.Background(), contract.NewJob{Ask: "post the weekly update", Why: "because the user asked"}); err != nil {
		t.Fatalf("creating the job failed: %v", err)
	}
	deps.Jobs = jobs

	registry := command.NewRegistry()
	for _, one := range command.New(registry, deps).All() {
		if err := registry.Register(one); err != nil {
			t.Fatalf("registering %s failed: %v", one.Name, err)
		}
	}
	if _, err := registry.Run(context.Background(), "/pause", contract.CommandContext{Channel: terminalChannel()}); err != nil {
		t.Fatalf("the pause command failed: %v", err)
	}
	return registry
}

func TestResumeSaysWhenNothingCanStartAJobAgain(t *testing.T) {
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(theStartOfTime))
	registry := oneRunningJobPaused(t, command.Deps{}, jobs)
	where := contract.CommandContext{Channel: terminalChannel()}

	if _, err := registry.Run(context.Background(), "/resume", where); err == nil {
		t.Fatalf("the resume command claimed to start a job again with nothing wired up to do it")
	}
	if _, err := registry.Run(context.Background(), "/resume", where); err == nil {
		t.Fatalf("the resume command forgot the job it had paused")
	}
}

func TestResumeKeepsAJobPausedWhenStartingItAgainFails(t *testing.T) {
	jobs := testkit.NewFakeJob(testkit.NewFakeClock(theStartOfTime))
	registry := oneRunningJobPaused(t, command.Deps{
		ResumeJob: func(context.Context, string) error { return errBrokenJobStore },
	}, jobs)

	_, err := registry.Run(context.Background(), "/resume", contract.CommandContext{Channel: terminalChannel()})
	if !errors.Is(err, errBrokenJobStore) {
		t.Errorf("the resume command hid what starting the job again failed with: %v", err)
	}
	listed, err := jobs.List(context.Background())
	if err != nil {
		t.Fatalf("listing the jobs failed: %v", err)
	}
	if listed[0].State != contract.JobPaused {
		t.Errorf("the job came out %s after the resume failed, and it should still be paused", listed[0].State)
	}
}

func TestDenyHandsBackWhatTheAnswerFailedWith(t *testing.T) {
	broken := errors.New("there is no preview numbered 3, so run /status to see what is waiting")
	_, err := answerOf(t, command.Deps{
		Answer: func(context.Context, string, contract.PreviewAnswer, string) error { return broken },
	}, "/deny 3 the post is wrong")
	if !errors.Is(err, broken) {
		t.Errorf("the deny command hid what the answer failed with: %v", err)
	}
}

func TestNewSaysWhatStartingASessionFailedWith(t *testing.T) {
	broken := errors.New("a session could not be written, so check that the database is writable")
	_, err := answerOf(t, command.Deps{
		NewSession: func(context.Context) (string, error) { return "", broken },
	}, "/new")
	if !errors.Is(err, broken) {
		t.Errorf("the new command hid what starting a session failed with: %v", err)
	}
}

func TestSessionsSaysWhatTheListingFailedWith(t *testing.T) {
	broken := errors.New("the sessions could not be read, so check that the database is readable")
	_, err := answerOf(t, command.Deps{
		Sessions: func(context.Context) ([]command.Session, error) { return nil, broken },
	}, "/sessions")
	if !errors.Is(err, broken) {
		t.Errorf("the sessions command hid what the listing failed with: %v", err)
	}
}

func TestSessionsSaysWhenNothingCanSwitch(t *testing.T) {
	_, err := answerOf(t, command.Deps{
		Sessions: func(context.Context) ([]command.Session, error) { return threeSessions(), nil },
	}, "/sessions 12")
	if err == nil {
		t.Fatalf("the sessions command claimed to switch with nothing wired up to switch")
	}
}

func TestSessionsNamesTheOneYouAreInWhenItIsTheOnlyOne(t *testing.T) {
	answer, err := answerOf(t, command.Deps{
		Sessions: func(context.Context) ([]command.Session, error) {
			return []command.Session{{ID: "12", Title: "the only conversation", Started: theStartOfTime, Current: true}}, nil
		},
	}, "/sessions")
	if err != nil {
		t.Fatalf("the sessions command failed: %v", err)
	}
	if !strings.Contains(answer, "/sessions 12") {
		t.Errorf("the sessions command does not say how to switch when there is only one: %q", answer)
	}
}

func TestUndoRestoresAFileWhoseEventRecordedNoMode(t *testing.T) {
	folder := t.TempDir()
	notes := writeFile(t, folder, "notes.md", "the words the agent wrote")
	store := testkit.NewFakeStore()
	appendMessage(t, store, "rewrite the notes")
	appendFileChange(t, store, contract.FileChangeBody{Path: notes, Existed: true, PriorContents: []byte("the words the user wrote")})

	runOne(t, command.Deps{Store: store}, "/undo")

	about, err := os.Stat(notes)
	if err != nil {
		t.Fatalf("reading the restored file failed: %v", err)
	}
	if about.Mode().Perm() != contract.DataFileMode {
		t.Errorf("the restored file has mode %04o rather than the ordinary file mode", about.Mode().Perm())
	}
}

func TestUndoSaysWhenAFileWillNotGoBack(t *testing.T) {
	store := testkit.NewFakeStore()
	appendMessage(t, store, "write the notes")
	appendFileChange(t, store, contract.FileChangeBody{
		Path: filepath.Join(t.TempDir(), "gone", "notes.md"), Existed: true, PriorContents: []byte("what it held"),
	})

	_, err := answerOf(t, command.Deps{Store: store}, "/undo")
	if err == nil {
		t.Fatalf("the undo command said all was well when the folder was not there")
	}
	if !strings.Contains(err.Error(), "notes.md") {
		t.Errorf("the failure does not name the file that would not go back: %v", err)
	}
}

func TestUndoSaysWhenAFileChangeCannotBeRead(t *testing.T) {
	store := testkit.NewFakeStore()
	appendMessage(t, store, "write the notes")
	if _, err := store.Append(context.Background(), contract.Event{
		TaskID: "17", Kind: contract.EventFileChange, Body: json.RawMessage("not json at all"),
	}); err != nil {
		t.Fatalf("appending the damaged event failed: %v", err)
	}

	_, err := answerOf(t, command.Deps{Store: store}, "/undo")
	if err == nil {
		t.Fatalf("the undo command carried on past a file-change event it could not read")
	}
	if !strings.Contains(err.Error(), "nothing was put back") {
		t.Errorf("the failure does not say that nothing was put back: %v", err)
	}
}

func TestInstallAndUninstallNeedSomewhereToPrint(t *testing.T) {
	service := command.Service{Home: testkit.NewTempHome(t)}
	if err := service.Install(context.Background()); err == nil {
		t.Errorf("coeus install ran with nowhere to print what it did")
	}
	if err := service.Uninstall(context.Background(), nil); err == nil {
		t.Errorf("coeus uninstall ran with nowhere to print what it did")
	}
}

func TestInstallNeedsToKnowWhichBinaryTheServiceRuns(t *testing.T) {
	home := testkit.NewTempHome(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), ".config"))
	fakeSystemctl(t, 0)

	err := command.Service{Home: home, Output: &strings.Builder{}}.Install(context.Background())
	if err == nil {
		t.Fatalf("coeus install made a link to nothing at all")
	}
	if !strings.Contains(err.Error(), "running program") {
		t.Errorf("the refusal does not say what was missing: %v", err)
	}
}

func TestUninstallOnAMachineWithNoUnitSaysWhatItDid(t *testing.T) {
	home := testkit.NewTempHome(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), ".config"))
	fakeSystemctl(t, 1)
	written := &strings.Builder{}

	if err := aServiceIn(t, home, "", written).Uninstall(context.Background(), nil); err != nil {
		t.Fatalf("coeus uninstall failed on a machine with no unit: %v", err)
	}
	if !strings.Contains(written.String(), "systemctl") {
		t.Errorf("coeus uninstall does not say what the service manager refused:\n%s", written)
	}
	if !strings.Contains(written.String(), "kept") {
		t.Errorf("coeus uninstall does not say that it kept the home folder:\n%s", written)
	}
}

func TestUninstallPurgeNeedsSomebodyToAsk(t *testing.T) {
	home := testkit.NewTempHome(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), ".config"))
	fakeSystemctl(t, 0)

	err := command.Service{Home: home, Output: &strings.Builder{}}.Uninstall(context.Background(), []string{"--purge"})
	if err == nil {
		t.Fatalf("coeus uninstall --purge removed the home folder with nobody to ask")
	}
	if _, statErr := os.Stat(home.Root); statErr != nil {
		t.Errorf("the home folder was removed without anybody typing the word: %v", statErr)
	}
}

func TestUnitPathSaysWhenThereIsNoConfigurationFolder(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")

	if _, err := command.UnitPath(); err == nil {
		t.Fatalf("the unit path was worked out with no home directory and no XDG_CONFIG_HOME")
	}
}
