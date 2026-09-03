package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installer copies scripts/install.sh into the fixture and returns where it put
// it, so that a test never runs the one in the checkout.
func installer(t *testing.T, into string) string {
	t.Helper()
	return copyScript(t, into, "scripts/install.sh")
}

func TestTheInstallerPrintsHowToUseItAndStops(t *testing.T) {
	folder := t.TempDir()
	script := installer(t, folder)

	printed, code := runScript(t, script, folder, cleanEnvironment(folder, folder), "--help")

	if code != 0 {
		t.Fatalf("--help exited %d, want 0. It printed:\n%s", code, printed)
	}
	requirePrinted(t, printed, "usage:", "--help")
	requirePrinted(t, printed, "--from", "--help lists every flag")
	requirePrinted(t, printed, "--version", "--help lists every flag")
}

func TestTheInstallerRefusesAFlagItDoesNotKnow(t *testing.T) {
	folder := t.TempDir()
	script := installer(t, folder)

	printed, code := runScript(t, script, folder, cleanEnvironment(folder, folder), "--wobble")

	if code == 0 {
		t.Fatalf("an unknown flag was accepted. It printed:\n%s", printed)
	}
	requirePrinted(t, printed, "--wobble", "the refusal names the flag it did not know")
	requirePrinted(t, printed, "--help", "the refusal says what to do")
	if _, err := os.Stat(filepath.Join(folder, ".coeus")); err == nil {
		t.Error("a bad command line still made a home folder; nothing may be touched before the flags are read")
	}
}

func TestTheInstallerRefusesAnArchiveThatIsNotThere(t *testing.T) {
	fakes := newInstallerFakes(t, ubuntuTwentyFour)
	script := installer(t, fakes.fakes+".scripts")
	missing := filepath.Join(fakes.home, "nowhere", "coeus.tar.gz")

	printed, code := runScript(t, script, fakes.home, fakes.environment(), "--from", missing)

	if code == 0 {
		t.Fatalf("an archive that is not there was accepted. It printed:\n%s", printed)
	}
	requirePrinted(t, printed, missing, "the refusal names the archive it could not find")
}

func TestTheInstallerRefusesAnArchiveWhoseChecksumDoesNotMatch(t *testing.T) {
	fakes := newInstallerFakes(t, ubuntuTwentyFour)
	script := installer(t, fakes.fakes+".scripts")
	archive := writeFixtureArchive(t, filepath.Join(fakes.home, "downloads"), "amd64")
	writeChecksumFile(t, archive, true)

	printed, code := runScript(t, script, fakes.home, fakes.environment(), "--from", archive)

	if code == 0 {
		t.Fatalf("an archive whose checksum was wrong was installed. It printed:\n%s", printed)
	}
	requirePrinted(t, printed, "checksum", "the refusal says what did not match")
	releases := filepath.Join(fakes.home, ".coeus", "releases", fixtureVersion)
	if _, err := os.Stat(releases); err == nil {
		t.Errorf("%s was unpacked even though its checksum was wrong", releases)
	}
}

func TestTheInstallerInstallsFromALocalArchiveAndRunsInit(t *testing.T) {
	fakes := newInstallerFakes(t, ubuntuTwentyFour)
	script := installer(t, fakes.fakes+".scripts")
	archive := writeFixtureArchive(t, filepath.Join(fakes.home, "downloads"), "amd64")
	writeChecksumFile(t, archive, false)

	printed, code := runScript(t, script, fakes.home, fakes.environment(),
		"--from", archive, "--", "--model", "local", "--yes")

	if code != 0 {
		t.Fatalf("the installer exited %d, want 0. It printed:\n%s", code, printed)
	}
	assertReleaseIsInPlace(t, fakes, printed)
	assertThePrivilegedStepsRan(t, fakes, printed)
	assertInitRanWithTheFlagsGiven(t, fakes, printed)
}

// assertReleaseIsInPlace checks the part of the job that is only about files:
// the unpacked release, the link that says which one is live, and the launcher.
func assertReleaseIsInPlace(t *testing.T, fakes installerFakes, printed string) {
	t.Helper()
	release := filepath.Join(fakes.home, ".coeus", "releases", fixtureVersion)
	for _, path := range []string{
		filepath.Join(release, "coeus"),
		filepath.Join(release, "node", "bin", "node"),
		filepath.Join(release, "worker", "browser", "dist", "main.js"),
		filepath.Join(release, "worker", "desktop", "dist", "main.js"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s is not there after the install: %v\nThe installer printed:\n%s", path, err, printed)
		}
	}

	link := filepath.Join(fakes.home, ".coeus", "releases", "current")
	pointsAt, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("%s is not a link: %v", link, err)
	}
	if pointsAt != filepath.Join(release, "coeus") {
		t.Errorf("%s points at %s, want the binary of the release just installed at %s",
			link, pointsAt, filepath.Join(release, "coeus"))
	}

	launcher := filepath.Join(fakes.home, ".local", "bin", "coeus")
	if _, err := os.Lstat(launcher); err != nil {
		t.Errorf("no launcher at %s, so nothing put coeus on the PATH: %v", launcher, err)
	}
	if _, err := os.Stat(filepath.Join(fakes.home, "coeus")); err != nil {
		t.Errorf("the work folder ~/coeus was not made: %v", err)
	}
	requirePrinted(t, printed, "Chrome", "the user is told that Chrome is theirs to install")
}

// assertThePrivilegedStepsRan checks the steps that need administrator rights on
// a real machine and that the fakes stood in for here.
func assertThePrivilegedStepsRan(t *testing.T, fakes installerFakes, printed string) {
	t.Helper()
	asked := fakes.asked(t)
	for _, want := range []string{"apt-get", "bubblewrap", "ripgrep", "apparmor_parser", "bwrap"} {
		if !strings.Contains(asked, want) {
			t.Errorf("nothing asked the machine for %q. The fakes were asked:\n%s\nThe installer printed:\n%s",
				want, asked, printed)
		}
	}

	profile := filepath.Join(fakes.apparmor, "bwrap")
	content := readFile(t, profile)
	for _, want := range []string{"abi <abi/4.0>", "include <tunables/global>", "/usr/bin/bwrap", "userns"} {
		if !strings.Contains(content, want) {
			t.Errorf("the AppArmor profile is missing %q. It says:\n%s", want, content)
		}
	}
	requirePrinted(t, printed, "Ubuntu", "the installer says which distribution it found")
}

// assertInitRanWithTheFlagsGiven checks the last step: coeus init runs through
// the launcher and is handed everything written after the two dashes.
func assertInitRanWithTheFlagsGiven(t *testing.T, fakes installerFakes, printed string) {
	t.Helper()
	ran := readFile(t, fakes.initLog)
	if !strings.Contains(ran, "init") {
		t.Errorf("coeus init was not run. The stub was called with:\n%s\nThe installer printed:\n%s", ran, printed)
	}
	for _, want := range []string{"--model local", "--yes"} {
		if !strings.Contains(ran, want) {
			t.Errorf("init was not given %q. It was called with:\n%s", want, ran)
		}
	}
}

func TestTheInstallerWritesNoAppArmorProfileOnDebian(t *testing.T) {
	fakes := newInstallerFakes(t, debianTwelve)
	script := installer(t, fakes.fakes+".scripts")
	archive := writeFixtureArchive(t, filepath.Join(fakes.home, "downloads"), "amd64")
	writeChecksumFile(t, archive, false)

	printed, code := runScript(t, script, fakes.home, fakes.environment(), "--from", archive, "--", "--yes")

	if code != 0 {
		t.Fatalf("the installer exited %d on Debian, want 0. It printed:\n%s", code, printed)
	}
	if _, err := os.Stat(filepath.Join(fakes.apparmor, "bwrap")); err == nil {
		t.Error("an AppArmor profile was written on Debian, which does not block unprivileged user namespaces")
	}
	requirePrinted(t, printed, "Debian", "the installer says which distribution it found")
}

func TestTheInstallerSaysWhatToDoOnADistributionItDoesNotKnow(t *testing.T) {
	fakes := newInstallerFakes(t, fedora)
	script := installer(t, fakes.fakes+".scripts")
	archive := writeFixtureArchive(t, filepath.Join(fakes.home, "downloads"), "amd64")
	writeChecksumFile(t, archive, false)

	printed, code := runScript(t, script, fakes.home, fakes.environment(), "--from", archive, "--", "--yes")

	if code != 0 {
		t.Fatalf("the installer exited %d on Fedora, want 0: an unknown distribution is not a failure. It printed:\n%s",
			code, printed)
	}
	requirePrinted(t, printed, "fedora", "the message names the distribution it does not know")
	for _, want := range []string{"bubblewrap", "ripgrep"} {
		requirePrinted(t, printed, want, "the message names the packages to install by hand")
	}
	if strings.Contains(fakes.asked(t), "apt-get") {
		t.Error("apt-get was run on Fedora, where it does not exist")
	}
	if _, err := os.Stat(filepath.Join(fakes.home, ".coeus", "releases", fixtureVersion)); err != nil {
		t.Errorf("the release was not installed on an unknown distribution: %v", err)
	}
}

func TestTheInstallerCanBeToldToLeaveSignalAlone(t *testing.T) {
	fakes := newInstallerFakes(t, ubuntuTwentyFour)
	script := installer(t, fakes.fakes+".scripts")
	archive := writeFixtureArchive(t, filepath.Join(fakes.home, "downloads"), "amd64")
	writeChecksumFile(t, archive, false)

	printed, code := runScript(t, script, fakes.home, fakes.environment(),
		"--from", archive, "--no-signal", "--", "--yes")

	if code != 0 {
		t.Fatalf("--no-signal exited %d, want 0. It printed:\n%s", code, printed)
	}
	requirePrinted(t, printed, "signal-cli", "the installer says Signal was left out")
	if strings.Contains(fakes.asked(t), "signal-cli") {
		t.Errorf("something still went looking for signal-cli. The fakes were asked:\n%s", fakes.asked(t))
	}
}

func TestTheInstallerFallsBackToThePinnedSignalDownload(t *testing.T) {
	fakes := newInstallerFakes(t, ubuntuTwentyFour)
	// This package manager has bubblewrap and ripgrep but has never heard of
	// signal-cli, which is true of both Ubuntu and Debian today.
	writeExecutable(t, filepath.Join(fakes.fakes, "apt-get"),
		"#!/bin/sh\nprintf 'apt-get %s\\n' \"$*\" >> "+fakes.log+"\n"+
			"case \"$*\" in *signal-cli*) echo 'E: Unable to locate package signal-cli' >&2; exit 100 ;; esac\nexit 0\n")
	// A download that arrives but is not what was pinned, which is the case that
	// must never leave an unverified program on the machine.
	writeExecutable(t, filepath.Join(fakes.fakes, "curl"),
		"#!/bin/sh\nprintf 'curl %s\\n' \"$*\" >> "+fakes.log+"\n"+
			"for word in \"$@\"; do case \"$last\" in -o) echo 'not signal-cli' > \"$word\" ;; esac; last=\"$word\"; done\nexit 0\n")
	script := installer(t, fakes.fakes+".scripts")
	archive := writeFixtureArchive(t, filepath.Join(fakes.home, "downloads"), "amd64")
	writeChecksumFile(t, archive, false)

	printed, code := runScript(t, script, fakes.home, fakes.environment(), "--from", archive, "--", "--yes")

	if code != 0 {
		t.Fatalf("a package manager without signal-cli made the whole install fail with %d. It printed:\n%s", code, printed)
	}
	asked := fakes.asked(t)
	if !strings.Contains(asked, "signal-cli") || !strings.Contains(asked, "github.com") {
		t.Errorf("nothing tried the pinned signal-cli download. The fakes were asked:\n%s", asked)
	}
	requirePrinted(t, printed, "checksum", "a download that did not verify is named for what it was")
	requirePrinted(t, printed, "coeus doctor", "the warning says how to find out what is missing later")
	if _, err := os.Stat(filepath.Join(fakes.home, ".coeus", "tools", "signal-cli")); err == nil {
		t.Error("a signal-cli whose checksum was wrong was installed anyway")
	}
}

func TestTheInstallerIsShortEnoughToRead(t *testing.T) {
	path := filepath.Join(repositoryRoot(t), "scripts", "install.sh")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the installer: %v", err)
	}
	lines := strings.Count(string(content), "\n")
	if lines > maxInstallerLines {
		t.Errorf("scripts/install.sh is %d lines, and the brief caps it at %d; take something out rather than raising the cap",
			lines, maxInstallerLines)
	}
}

// maxInstallerLines is the cap brief 6.2 puts on the installer, which is there so
// that a person can read the whole thing before running it.
const maxInstallerLines = 200
