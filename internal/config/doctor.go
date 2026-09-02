package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// HealthProbeWait is how long the doctor waits for the local model daemon to
// answer its health check before deciding that it is not there. The daemon is on
// this machine, so an answer arrives at once or not at all.
const HealthProbeWait = 2 * time.Second

// Doctor looks at a home folder and reports what it finds: which parts of the
// layout are there, which have a mode that lets other accounts read them,
// whether the configuration loads, which of the outside programs Coeus uses are
// on the PATH, and whether the local model daemon answers its health check. It
// changes nothing at all, so it is safe to run at any time; "coeus init" and
// "coeus doctor" both print what it found.
func Doctor(ctx context.Context, home contract.Home) Report {
	report := Report{Root: home.Root}
	report.Findings = append(report.Findings, folderFindings(home)...)
	report.Findings = append(report.Findings, fileFindings(home)...)

	settings, err := Load(home)
	report.Findings = append(report.Findings, configurationFinding(home, settings, err))
	report.Findings = append(report.Findings, programFindings()...)
	report.Findings = append(report.Findings, daemonFinding(ctx, settings, err))
	return report
}

// configurationFinding says whether the configuration file is there and whether
// it loads. A file that is not there is a warning rather than a problem, because
// the defaults are a working configuration and a fresh home has no file yet.
func configurationFinding(home contract.Home, settings contract.Config, loading error) Finding {
	const what = "config.toml"
	if _, err := os.Stat(home.ConfigFile()); errors.Is(err, os.ErrNotExist) {
		return Finding{What: what, Result: Warning,
			Detail: "is not there yet, so the built-in defaults are in use; run coeus init to write one"}
	}
	if loading != nil {
		return Finding{What: what, Result: Trouble, Detail: "will not load: " + loading.Error()}
	}
	return Finding{What: what, Result: Fine, Detail: whatTheConfigurationAsksFor(settings)}
}

// whatTheConfigurationAsksFor is the one line the doctor prints about a
// configuration that loads: the model it reaches for, how much of the
// ask-me-first list the user kept, and how many rules of their own they wrote.
func whatTheConfigurationAsksFor(settings contract.Config) string {
	return fmt.Sprintf("loads, and names %s as the model to use, keeps %d of the %d ask-me-first entries, and adds %d permission rule(s) of its own",
		settings.DefaultModel, len(settings.AskMeFirst), len(contract.DefaultAskMeFirst()), len(settings.PermissionRules))
}

// folderFindings looks at the home folder and every folder of the layout inside
// it: each one has to be there, and each one has to be readable by this account
// and by nobody else.
func folderFindings(home contract.Home) []Finding {
	findings := []Finding{}
	for _, folder := range home.Folders() {
		what := "the home folder"
		if folder != home.Root {
			what = filepath.Base(folder)
		}
		findings = append(findings, oneFolderFinding(what, folder))
	}
	return findings
}

// oneFolderFinding says whether one folder of the layout is there with the right
// mode.
func oneFolderFinding(what string, folder string) Finding {
	about, err := os.Stat(folder)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return Finding{What: what, Result: Trouble, Detail: "is not there, so run coeus init to make the home folder and everything in it"}
	case err != nil:
		return Finding{What: what, Result: Trouble, Detail: fmt.Sprintf("cannot be looked at: %v", err)}
	case !about.IsDir():
		return Finding{What: what, Result: Trouble, Detail: "is a file where a folder belongs, so move it aside and run coeus init"}
	case about.Mode().Perm() != contract.HomeFolderMode:
		return Finding{What: what, Result: Trouble, Detail: fmt.Sprintf(
			"has mode %04o, so other accounts on this machine can look inside it; run chmod 0700 on %s", about.Mode().Perm(), folder)}
	default:
		return Finding{What: what, Result: Fine, Detail: "is there, mode 0700"}
	}
}

// fileFindings looks at the three files a working home has that the layout does
// not make: the database and the two halves of the vault. A file that is not
// there yet is a warning, because the first run and "coeus init" make them.
func fileFindings(home contract.Home) []Finding {
	return []Finding{
		oneFileFinding("coeus.db", home.DatabaseFile(), contract.SecretFileMode, "the event log, the records, and the memory index live in it; the first run makes it"),
		oneFileFinding("vault.age", home.VaultFile(), contract.SecretFileMode, "there are no secrets stored yet; coeus init makes it"),
		oneFileFinding("vault.key", home.VaultKeyFile(), contract.SecretFileMode, "there is no key to the vault yet; coeus init makes it"),
	}
}

// oneFileFinding says whether one file is there and, when it is, whether its
// mode keeps it to this account.
func oneFileFinding(what string, path string, wanted fs.FileMode, whenMissing string) Finding {
	about, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return Finding{What: what, Result: Warning, Detail: "is not there: " + whenMissing}
	case err != nil:
		return Finding{What: what, Result: Trouble, Detail: fmt.Sprintf("cannot be looked at: %v", err)}
	case about.Mode().Perm() != wanted:
		return Finding{What: what, Result: Trouble, Detail: fmt.Sprintf(
			"has mode %04o, so other accounts on this machine can read it; run chmod %04o on %s", about.Mode().Perm(), wanted, path)}
	default:
		return Finding{What: what, Result: Fine, Detail: fmt.Sprintf("is there, mode %04o", wanted)}
	}
}

// outsideProgram is one program Coeus runs that it does not ship: the names to
// look for on the PATH, and what stops working when none of them is there.
type outsideProgram struct {
	names       []string
	whatItIsFor string
	whenMissing string
}

// theOutsidePrograms is every program Coeus calls out to. None of them stops
// Coeus from running, so a missing one is a warning that names what it switches
// off.
func theOutsidePrograms() []outsideProgram {
	return []outsideProgram{
		{names: []string{"signal-cli"}, whatItIsFor: "talking over Signal", whenMissing: "install signal-cli and run coeus signal link"},
		{names: []string{"bwrap"}, whatItIsFor: "the sandbox every shell command runs in", whenMissing: "install bubblewrap, or the shell tool stays switched off"},
		{names: []string{"rg"}, whatItIsFor: "searching files", whenMissing: "install ripgrep"},
		{names: []string{"google-chrome", "chromium"}, whatItIsFor: "the browser tools", whenMissing: "install Google Chrome or Chromium"},
		{names: []string{"node"}, whatItIsFor: "the browser and desktop workers", whenMissing: "install Node"},
	}
}

// programFindings looks for each outside program on the PATH.
func programFindings() []Finding {
	findings := []Finding{}
	for _, program := range theOutsidePrograms() {
		findings = append(findings, oneProgramFinding(program))
	}
	return findings
}

// oneProgramFinding looks for one program under each of the names it goes by.
func oneProgramFinding(program outsideProgram) Finding {
	what := strings.Join(program.names, " or ")
	for _, name := range program.names {
		if found, err := exec.LookPath(name); err == nil {
			return Finding{What: what, Result: Fine, Detail: "is at " + found}
		}
	}
	return Finding{What: what, Result: Warning, Detail: fmt.Sprintf(
		"is not on your PATH, so %s is switched off; %s", program.whatItIsFor, program.whenMissing)}
}

// daemonFinding asks the local model daemon whether it is well. The address
// comes from the configuration, so a configuration that will not load leaves
// nothing to ask.
func daemonFinding(ctx context.Context, settings contract.Config, loading error) Finding {
	const what = "the local model daemon"
	if loading != nil {
		return Finding{What: what, Result: Warning, Detail: "was not asked, because the configuration has to load before its address is known"}
	}
	address := healthAddress(localBaseAddress(settings))
	if address == "" {
		return Finding{What: what, Result: Warning, Detail: fmt.Sprintf(
			"was not asked, because no model alias called %q points at a server on this machine", contract.LocalModelAlias)}
	}
	if detail, well := askTheDaemon(ctx, address); !well {
		return Finding{What: what, Result: Warning, Detail: detail}
	}
	return Finding{What: what, Result: Fine, Detail: "answered its health check at " + address}
}

// askTheDaemon makes one read-only request to a health endpoint and says whether
// it answered properly, and what to say when it did not.
func askTheDaemon(ctx context.Context, address string) (string, bool) {
	asking, cancel := context.WithTimeout(ctx, HealthProbeWait)
	defer cancel()

	request, err := http.NewRequestWithContext(asking, http.MethodGet, address, nil)
	if err != nil {
		return fmt.Sprintf("cannot be asked at %s: %v", address, err), false
	}
	asker := &http.Client{Timeout: HealthProbeWait}
	answer, err := asker.Do(request)
	if err != nil {
		return fmt.Sprintf("did not answer at %s, so start it with the command in CLAUDE.md and try again", address), false
	}
	defer func() { _ = answer.Body.Close() }()
	if answer.StatusCode != http.StatusOK {
		return fmt.Sprintf("answered %s at %s rather than a health report, so check that the address points at the daemon",
			answer.Status, address), false
	}
	return "", true
}

// localBaseAddress is the address of the model alias that stands for the daemon
// on this machine, or an empty string when the configuration names no such
// alias.
func localBaseAddress(settings contract.Config) string {
	for _, alias := range settings.Models {
		if alias.Name == contract.LocalModelAlias && alias.Provider == contract.ProviderOpenAI {
			return alias.BaseAddress
		}
	}
	return ""
}

// healthAddress turns the base address of an OpenAI-compatible server into the
// address of its health check, which sits beside the API rather than inside it:
// the daemon answers /health while the API lives under /v1.
func healthAddress(base string) string {
	if base == "" {
		return ""
	}
	trimmed := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(base), "/"), "/v1")
	return strings.TrimSuffix(trimmed, "/") + "/health"
}
