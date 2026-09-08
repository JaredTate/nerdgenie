package config_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/config"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// findingAbout picks one line out of a report by the name of the thing it looked
// at, and fails the test when the doctor did not look at it at all.
func findingAbout(t testing.TB, report config.Report, what string) config.Finding {
	t.Helper()
	for _, finding := range report.Findings {
		if finding.What == what {
			return finding
		}
	}
	t.Fatalf("the doctor said nothing about %q; it looked at %v", what, everythingLookedAt(report))
	return config.Finding{}
}

// everythingLookedAt names every check in a report, for a failure message.
func everythingLookedAt(report config.Report) []string {
	names := []string{}
	for _, finding := range report.Findings {
		names = append(names, finding.What)
	}
	return names
}

// putProgramsOnThePath makes an empty executable for each name in a folder of
// its own and points the PATH at it, so that a test can say exactly which
// outside programs this machine appears to have.
func putProgramsOnThePath(t testing.TB, names ...string) {
	t.Helper()
	folder := t.TempDir()
	for _, name := range names {
		path := filepath.Join(folder, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("cannot make the pretend program %s: %v", name, err)
		}
	}
	t.Setenv("PATH", folder)
}

// theFivePrograms is what the doctor looks for on the PATH, under the names it
// reports them by.
var theFivePrograms = []string{"signal-cli", "bwrap", "rg", "Chrome", "node"}

func TestTheDoctorOnAnEmptyHomeFindsTheFoldersAndMissesTheRest(t *testing.T) {
	home := testkit.NewTempHome(t)

	report := config.Doctor(context.Background(), home)

	if report.Root != home.Root {
		t.Errorf("the report is about %q, want the home folder %q", report.Root, home.Root)
	}
	if finding := findingAbout(t, report, "the home folder"); finding.Result != config.Fine {
		t.Errorf("the home folder is reported %s: %s, want it fine", finding.Result, finding.Detail)
	}
	if finding := findingAbout(t, report, "persona"); finding.Result != config.Fine {
		t.Errorf("the persona folder is reported %s: %s, want it fine", finding.Result, finding.Detail)
	}
	for _, missing := range []string{"config.toml", "nerdgenie.db", "vault.age", "vault.key"} {
		if finding := findingAbout(t, report, missing); finding.Result != config.Warning {
			t.Errorf("%s is reported %s: %s, want a warning, because a fresh home has none of these yet",
				missing, finding.Result, finding.Detail)
		}
	}
	if report.Verdict() != config.Warning {
		t.Errorf("the verdict on an empty home is %s, want a warning: nothing is broken and much is missing", report.Verdict())
	}
}

func TestTheDoctorOnAFullHomeFindsEverything(t *testing.T) {
	home := buildFullHome(t)
	putProgramsOnThePath(t, "signal-cli", "bwrap", "rg", "google-chrome", "node")

	report := config.Doctor(context.Background(), home)

	for _, what := range append([]string{"the home folder", "config.toml", "nerdgenie.db", "vault.age", "vault.key"}, theFivePrograms...) {
		if finding := findingAbout(t, report, what); finding.Result != config.Fine {
			t.Errorf("%s is reported %s: %s, want it fine on a home that has everything", what, finding.Result, finding.Detail)
		}
	}
}

func TestTheDoctorReportsAFolderThatIsNotThere(t *testing.T) {
	home := testkit.NewTempHome(t)
	if err := os.RemoveAll(home.SkillsFolder()); err != nil {
		t.Fatalf("cannot take the skills folder away for the test: %v", err)
	}

	report := config.Doctor(context.Background(), home)

	finding := findingAbout(t, report, "skills")
	if finding.Result != config.Trouble {
		t.Errorf("a missing folder is reported %s, want it a problem", finding.Result)
	}
	if !strings.Contains(finding.Detail, "nerdgenie init") {
		t.Errorf("the detail is %q, want it to say that nerdgenie init makes the folder", finding.Detail)
	}
	if report.Verdict() != config.Trouble {
		t.Errorf("the verdict is %s, want a problem when a folder of the layout is missing", report.Verdict())
	}
}

func TestTheDoctorReportsAFolderAnyoneCanRead(t *testing.T) {
	home := testkit.NewTempHome(t)
	if err := os.Chmod(home.BrowserFolder(), 0o755); err != nil {
		t.Fatalf("cannot loosen the browser folder for the test: %v", err)
	}

	finding := findingAbout(t, config.Doctor(context.Background(), home), "browser")
	if finding.Result != config.Trouble {
		t.Errorf("a folder anyone can read is reported %s, want it a problem", finding.Result)
	}
	if !strings.Contains(finding.Detail, "0700") {
		t.Errorf("the detail is %q, want it to name the mode the folder should have", finding.Detail)
	}
}

func TestTheDoctorReportsAVaultKeyAnyoneCanRead(t *testing.T) {
	home := buildFullHome(t)
	if err := os.Chmod(home.VaultKeyFile(), 0o644); err != nil {
		t.Fatalf("cannot loosen the vault key for the test: %v", err)
	}

	finding := findingAbout(t, config.Doctor(context.Background(), home), "vault.key")
	if finding.Result != config.Trouble {
		t.Errorf("a vault key anyone can read is reported %s, want it a problem", finding.Result)
	}
	if !strings.Contains(finding.Detail, "0600") {
		t.Errorf("the detail is %q, want it to name the mode the key should have", finding.Detail)
	}
}

func TestTheDoctorReportsAConfigurationThatWillNotLoad(t *testing.T) {
	home := writeConfig(t, "\n[caps]\nrounds_per_task = -1\n")

	finding := findingAbout(t, config.Doctor(context.Background(), home), "config.toml")
	if finding.Result != config.Trouble {
		t.Errorf("a configuration that will not load is reported %s, want it a problem", finding.Result)
	}
	if !strings.Contains(finding.Detail, "caps.rounds_per_task") {
		t.Errorf("the detail is %q, want it to carry the problem the loader found", finding.Detail)
	}
}

func TestTheDoctorReportsEveryProgramThatIsNotOnThePath(t *testing.T) {
	home := testkit.NewTempHome(t)
	putProgramsOnThePath(t)

	report := config.Doctor(context.Background(), home)

	for _, program := range theFivePrograms {
		finding := findingAbout(t, report, program)
		if finding.Result != config.Warning {
			t.Errorf("%s is missing and is reported %s, want a warning saying what is switched off", program, finding.Result)
		}
		if len(strings.Fields(finding.Detail)) < 5 {
			t.Errorf("the detail for %s is %q, want a sentence saying what is switched off without it", program, finding.Detail)
		}
	}
}

func TestTheDoctorFindsAChromiumWhereThereIsNoChrome(t *testing.T) {
	home := testkit.NewTempHome(t)
	putProgramsOnThePath(t, "chromium")

	finding := findingAbout(t, config.Doctor(context.Background(), home), "Chrome")
	if finding.Result != config.Fine {
		t.Errorf("chromium alone is reported %s: %s, want it fine, because either browser will do", finding.Result, finding.Detail)
	}
}

// TestTheDoctorWarnsWhenTheOnlyChromeIsAScriptThatForcesHeadless is jared-rosie
// on 7 September 2026: the first google-chrome on the PATH was a wrapper script
// from another project forcing --headless=new, every Chrome the agent started
// had no window, and the doctor reported the wrapper as fine.
func TestTheDoctorWarnsWhenTheOnlyChromeIsAScriptThatForcesHeadless(t *testing.T) {
	home := testkit.NewTempHome(t)
	folder := t.TempDir()
	launcher := filepath.Join(folder, "google-chrome")
	wrapper := "#!/bin/sh\nexec nice -n 15 taskset -c 0-15 /usr/bin/google-chrome-stable --headless=new --disable-gpu \"$@\"\n"
	if err := os.WriteFile(launcher, []byte(wrapper), 0o755); err != nil {
		t.Fatalf("cannot write the pretend launcher: %v", err)
	}
	t.Setenv("PATH", folder)

	finding := findingAbout(t, config.Doctor(context.Background(), home), "Chrome")
	if finding.Result != config.Warning {
		t.Errorf("a Chrome that is only a launcher forcing headless is reported %s: %s, want a warning", finding.Result, finding.Detail)
	}
	if !strings.Contains(finding.Detail, launcher) || !strings.Contains(finding.Detail, "headless") {
		t.Errorf("the detail reads %q and does not name the launcher at %s and say that it forces headless", finding.Detail, launcher)
	}
	if !strings.Contains(finding.Detail, "install") {
		t.Errorf("the detail reads %q and does not say what to install", finding.Detail)
	}
}

func TestTheDoctorSaysTheLocalModelDaemonAnswered(t *testing.T) {
	daemon := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/health" {
			http.Error(writer, "this fake daemon serves only /health", http.StatusNotFound)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		if _, err := writer.Write([]byte(`{"status":"ok"}`)); err != nil {
			t.Errorf("the fake daemon could not answer: %v", err)
		}
	}))
	defer daemon.Close()
	home := writeConfig(t, localAliasAt(daemon.URL+"/v1"))

	finding := findingAbout(t, config.Doctor(context.Background(), home), "the local model daemon")
	if finding.Result != config.Fine {
		t.Errorf("a daemon that answers is reported %s: %s, want it fine", finding.Result, finding.Detail)
	}
}

func TestTheDoctorSaysTheLocalModelDaemonIsNotThere(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	address := closed.URL
	closed.Close()
	home := writeConfig(t, localAliasAt(address+"/v1"))

	finding := findingAbout(t, config.Doctor(context.Background(), home), "the local model daemon")
	if finding.Result != config.Warning {
		t.Errorf("a daemon on a closed port is reported %s: %s, want a warning saying how to start it", finding.Result, finding.Detail)
	}
	if !strings.Contains(finding.Detail, address) {
		t.Errorf("the detail is %q, want it to name the address that did not answer", finding.Detail)
	}
}

func TestTheDoctorSaysWhenTheLocalModelDaemonAnswersWithSomethingElse(t *testing.T) {
	daemon := testkit.NewFakeProviderServer(testkit.Script{Name: "the doctor never asks this server for a reply"})
	defer daemon.Close()
	home := writeConfig(t, localAliasAt(daemon.Address()+"/v1"))

	finding := findingAbout(t, config.Doctor(context.Background(), home), "the local model daemon")
	if finding.Result != config.Warning {
		t.Errorf("a server with no health check is reported %s: %s, want a warning", finding.Result, finding.Detail)
	}
}

func TestTheDoctorSkipsTheDaemonWhenTheConfigurationWillNotLoad(t *testing.T) {
	home := writeConfig(t, "nosuchkey = 1\n")

	finding := findingAbout(t, config.Doctor(context.Background(), home), "the local model daemon")
	if finding.Result != config.Warning {
		t.Errorf("the daemon check is reported %s, want a warning saying it could not be made", finding.Result)
	}
	if !strings.Contains(finding.Detail, "configuration") {
		t.Errorf("the detail is %q, want it to say the configuration has to load first", finding.Detail)
	}
}

func TestThePrintedReportNamesTheRootEveryCheckAndTheVerdict(t *testing.T) {
	home := buildFullHome(t)
	putProgramsOnThePath(t, "signal-cli", "bwrap", "rg", "google-chrome", "node")

	report := config.Doctor(context.Background(), home)
	printed := report.String()

	if !strings.Contains(printed, home.Root) {
		t.Errorf("the printed report does not name the home folder %q:\n%s", home.Root, printed)
	}
	for _, finding := range report.Findings {
		if !strings.Contains(printed, finding.What) {
			t.Errorf("the printed report leaves out the check %q:\n%s", finding.What, printed)
		}
	}
	if !strings.Contains(printed, string(report.Verdict())) {
		t.Errorf("the printed report does not carry the verdict %q:\n%s", report.Verdict(), printed)
	}
	if lines := strings.Count(printed, "\n"); lines < len(report.Findings) {
		t.Errorf("the printed report has %d lines and there are %d checks, so it does not print one line each", lines, len(report.Findings))
	}
}

func TestTheVerdictIsTheWorstOfTheFindings(t *testing.T) {
	forEachSet := []struct {
		what     string
		findings []config.Finding
		want     config.Result
	}{
		{"nothing at all", nil, config.Fine},
		{"all fine", []config.Finding{{Result: config.Fine}, {Result: config.Fine}}, config.Fine},
		{"one warning", []config.Finding{{Result: config.Fine}, {Result: config.Warning}}, config.Warning},
		{"one problem", []config.Finding{{Result: config.Warning}, {Result: config.Trouble}}, config.Trouble},
	}
	for _, set := range forEachSet {
		t.Run(set.what, func(t *testing.T) {
			report := config.Report{Root: "/somewhere", Findings: set.findings}
			if report.Verdict() != set.want {
				t.Errorf("the verdict on %s is %s, want %s", set.what, report.Verdict(), set.want)
			}
			if report.String() == "" {
				t.Error("the printed report is empty, and a person has to be able to read it")
			}
		})
	}
}

// localAliasAt is a configuration whose one model alias is the local daemon at
// an address the test chose.
func localAliasAt(address string) string {
	return "[[models]]\nname = \"local\"\nprovider = \"openai\"\nbase_address = \"" + address +
		"\"\nmodel_name = \"local-coder\"\ncontext_length = 262144\n"
}

// buildFullHome makes a home folder with everything in it: a configuration that
// loads, a database file, and a vault with the modes the layout calls for.
func buildFullHome(t testing.TB) contract.Home {
	t.Helper()
	home := testkit.NewTempHome(t)
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()

	files := map[string]os.FileMode{
		home.ConfigFile():   contract.DataFileMode,
		home.DatabaseFile(): contract.SecretFileMode,
		home.VaultFile():    contract.SecretFileMode,
		home.VaultKeyFile(): contract.SecretFileMode,
	}
	for path, mode := range files {
		content := []byte("a file the doctor only looks at\n")
		if path == home.ConfigFile() {
			content = []byte(localAliasAt(closed.URL + "/v1"))
		}
		if err := os.WriteFile(path, content, mode); err != nil {
			t.Fatalf("cannot make %s for the test: %v", path, err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatalf("cannot set the mode on %s: %v", path, err)
		}
	}
	return home
}

func TestTheDoctorReportsAFileWhereAFolderBelongs(t *testing.T) {
	home := testkit.NewTempHome(t)
	if err := os.RemoveAll(home.MemoryFolder()); err != nil {
		t.Fatalf("cannot take the memory folder away for the test: %v", err)
	}
	if err := os.WriteFile(home.MemoryFolder(), []byte("not a folder\n"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot put a file where the memory folder belongs: %v", err)
	}

	finding := findingAbout(t, config.Doctor(context.Background(), home), "memory")
	if finding.Result != config.Trouble {
		t.Errorf("a file where a folder belongs is reported %s, want it a problem", finding.Result)
	}
	if !strings.Contains(finding.Detail, "folder") {
		t.Errorf("the detail is %q, want it to say a folder belongs there", finding.Detail)
	}
}

func TestTheDoctorSkipsTheDaemonWhenNoAliasPointsAtThisMachine(t *testing.T) {
	home := writeConfig(t, strings.Join([]string{
		`default_model = "cloud"`,
		"",
		"[[models]]",
		`name = "cloud"`,
		`provider = "cli"`,
		`program = "claude"`,
		`model_name = "opus"`,
		"context_length = 200000",
		"",
	}, "\n"))

	finding := findingAbout(t, config.Doctor(context.Background(), home), "the local model daemon")
	if finding.Result != config.Warning {
		t.Errorf("a configuration with no local alias reports the daemon %s, want a warning", finding.Result)
	}
	if !strings.Contains(finding.Detail, contract.LocalModelAlias) {
		t.Errorf("the detail is %q, want it to name the alias it looked for", finding.Detail)
	}
}

func TestTheDoctorSaysWhatTheConfigurationAsksFor(t *testing.T) {
	home := buildFullHome(t)

	finding := findingAbout(t, config.Doctor(context.Background(), home), "config.toml")
	if finding.Result != config.Fine {
		t.Fatalf("the configuration is reported %s: %s, want it fine", finding.Result, finding.Detail)
	}
	for _, piece := range []string{contract.LocalModelAlias, "ask-me-first", "no permission rules"} {
		if !strings.Contains(finding.Detail, piece) {
			t.Errorf("the detail is %q, want it to mention %q so a person can see what the file asks for", finding.Detail, piece)
		}
	}
}

// TestTheDoctorSaysWhichWayTheSandboxSettingIsTurned holds the one line that
// tells a person whether commands are boxed into the sandbox roots or running on
// their machine as them. A home with no configuration file at all runs with the
// sandbox off, which is the default, and the report has to say so.
func TestTheDoctorSaysWhichWayTheSandboxSettingIsTurned(t *testing.T) {
	for _, written := range []struct {
		document string
		wanted   string
	}{
		{"", contract.SandboxOff},
		{"sandbox = \"off\"\n", contract.SandboxOff},
		{"sandbox = \"fence\"\n", contract.SandboxFence},
	} {
		home := writeConfig(t, written.document)
		finding := findingAbout(t, config.Doctor(context.Background(), home), "the sandbox setting")

		if finding.Result != config.Fine {
			t.Errorf("the configuration %q reports the sandbox %s: %s, want it fine either way",
				written.document, finding.Result, finding.Detail)
		}
		if !strings.Contains(finding.Detail, written.wanted) {
			t.Errorf("the configuration %q is reported as %q, and it does not name %q",
				written.document, finding.Detail, written.wanted)
		}
	}
}
