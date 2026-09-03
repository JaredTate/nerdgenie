package release

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The fixtures every installer test is built out of: a release archive holding a
// stub program, and the fake machine the installer runs against.

// fixtureVersion is the version the fixture archive claims to be. It is not a
// version this repository has ever tagged, so a test that found the real thing
// installed would notice.
const fixtureVersion = "9.9.9-fixture"

// installerFakes is everything a test needs to run the installer without
// administrator rights and without the network.
type installerFakes struct {
	// home is the temporary HOME, so that the real ~/.coeus is never touched.
	home string
	// fakes is the folder of fake programs that goes on the front of the PATH.
	fakes string
	// log is the file every fake program writes what it was asked to do into.
	log string
	// apparmor is the folder standing in for /etc/apparmor.d.
	apparmor string
	// osRelease is the file standing in for /etc/os-release.
	osRelease string
	// initLog is the file the stub program writes the arguments init got into.
	initLog string
}

// newInstallerFakes builds a fake machine running the distribution named, with a
// package manager, an AppArmor loader, and a sandbox probe that all say yes and
// write down what they were asked.
func newInstallerFakes(t *testing.T, osReleaseText string) installerFakes {
	t.Helper()
	root := t.TempDir()
	fakes := installerFakes{
		home:      filepath.Join(root, "home"),
		fakes:     filepath.Join(root, "bin"),
		log:       filepath.Join(root, "asked.log"),
		apparmor:  filepath.Join(root, "apparmor.d"),
		osRelease: filepath.Join(root, "os-release"),
		initLog:   filepath.Join(root, "init.log"),
	}
	if err := os.MkdirAll(fakes.home, 0o755); err != nil {
		t.Fatalf("cannot make the fake home folder: %v", err)
	}
	writeFile(t, fakes.osRelease, osReleaseText)

	// sudo runs whatever it was given, so that the privileged steps are exercised
	// without any privilege at all: everything they touch is inside the fixture.
	writeExecutable(t, filepath.Join(fakes.fakes, "sudo"),
		"#!/bin/sh\nprintf 'sudo %s\\n' \"$*\" >> "+fakes.log+"\nexec \"$@\"\n")
	writeExecutable(t, filepath.Join(fakes.fakes, "apt-get"), recordingProgram("apt-get", fakes.log, 0))
	writeExecutable(t, filepath.Join(fakes.fakes, "apparmor_parser"), recordingProgram("apparmor_parser", fakes.log, 0))
	writeExecutable(t, filepath.Join(fakes.fakes, "bwrap"), recordingProgram("bwrap", fakes.log, 0))
	writeExecutable(t, filepath.Join(fakes.fakes, "curl"), recordingProgram("curl", fakes.log, 1))
	return fakes
}

// environment is the environment the installer runs in on this fake machine.
func (fakes installerFakes) environment() []string {
	return cleanEnvironment(fakes.home, fakes.fakes,
		"COEUS_OS_RELEASE="+fakes.osRelease,
		"COEUS_APPARMOR_DIR="+fakes.apparmor,
		"COEUS_INIT_LOG="+fakes.initLog,
	)
}

// asked is everything the fake programs were asked to do, as one piece of text.
func (fakes installerFakes) asked(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile(fakes.log)
	if err != nil {
		return ""
	}
	return string(content)
}

// ubuntuTwentyFour is a machine AppArmor blocks unprivileged user namespaces on,
// which is the one case the installer has to write a profile for.
const ubuntuTwentyFour = "ID=ubuntu\nVERSION_ID=\"24.04\"\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\n"

// debianTwelve is a machine that needs no AppArmor profile.
const debianTwelve = "ID=debian\nVERSION_ID=\"12\"\nPRETTY_NAME=\"Debian GNU/Linux 12 (bookworm)\"\n"

// fedora is a distribution the installer does not know how to install packages
// on, and has to say so plainly rather than guess.
const fedora = "ID=fedora\nVERSION_ID=\"42\"\nPRETTY_NAME=\"Fedora Linux 42\"\n"

// writeFixtureArchive builds a release archive shaped exactly like the one
// scripts/release/build.sh writes, holding a stub program in place of the real
// binary, and returns its path. The stub writes the arguments it was given into
// the file COEUS_INIT_LOG names, so a test can prove that init was run and with
// what.
func writeFixtureArchive(t *testing.T, into string, architecture string) string {
	t.Helper()
	folder := "coeus-" + fixtureVersion + "-" + architecture
	staging := filepath.Join(t.TempDir(), folder)

	writeExecutable(t, filepath.Join(staging, "coeus"),
		"#!/bin/sh\nprintf 'coeus %s\\n' \"$*\" >> \"${COEUS_INIT_LOG:-/dev/null}\"\nexit 0\n")
	writeFile(t, filepath.Join(staging, "VERSION"), fixtureVersion+"\n")
	writeExecutable(t, filepath.Join(staging, "node", "bin", "node"), "#!/bin/sh\nexit 0\n")
	writeFile(t, filepath.Join(staging, "workers", "browser", "main.js"), "// the browser worker\n")
	writeFile(t, filepath.Join(staging, "workers", "desktop", "main.js"), "// the desktop worker\n")

	if err := os.MkdirAll(into, 0o755); err != nil {
		t.Fatalf("cannot make the folder the fixture archive goes in: %v", err)
	}
	archive := filepath.Join(into, folder+".tar.gz")
	packing := exec.Command("tar", "czf", archive, "-C", staging,
		"coeus", "VERSION", "node", "workers")
	if printed, err := packing.CombinedOutput(); err != nil {
		t.Fatalf("cannot pack the fixture archive: %v\n%s", err, printed)
	}
	return archive
}

// writeChecksumFile writes a SHA256SUMS file beside the archive, in the format
// sha256sum reads, so the installer has something to verify against. When wrong
// is true the checksum is deliberately not the archive's, which is how the test
// proves a bad download is refused.
func writeChecksumFile(t *testing.T, archive string, wrong bool) string {
	t.Helper()
	content, err := os.ReadFile(archive)
	if err != nil {
		t.Fatalf("cannot read the fixture archive to check it: %v", err)
	}
	sum := sha256.Sum256(content)
	written := hex.EncodeToString(sum[:])
	if wrong {
		written = "0000000000000000000000000000000000000000000000000000000000000000"
	}
	path := filepath.Join(filepath.Dir(archive), "SHA256SUMS")
	writeFile(t, path, written+"  "+filepath.Base(archive)+"\n")
	return path
}
