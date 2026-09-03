package release

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The release is three shell scripts and an installer, and none of them are Go,
// so the only honest way to test them is to run them. Every test in this package
// copies the script it is testing into a temporary folder, gives it a temporary
// HOME and a PATH with fakes on the front, and reads what it printed and what it
// left on disk. Nothing here ever touches the real home folder, the real package
// manager, or the real /etc.

// repositoryRoot is the checkout these tests read the scripts out of.
func repositoryRoot(t *testing.T) string {
	t.Helper()
	here, err := os.Getwd()
	if err != nil {
		t.Fatalf("cannot find the working directory the test started in: %v", err)
	}
	return filepath.Dir(filepath.Dir(here))
}

// runScript runs one shell script from the folder given and returns everything
// it printed on both streams together with its exit code, which is what a caller
// wants to assert on. A script that cannot be started at all fails the test.
func runScript(t *testing.T, script string, from string, environment []string, arguments ...string) (string, int) {
	t.Helper()
	command := exec.Command("/bin/sh", append([]string{script}, arguments...)...)
	command.Env = environment
	command.Dir = from
	printed, err := command.CombinedOutput()

	code := 0
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		code = exit.ExitCode()
	default:
		t.Fatalf("cannot run %s: %v", script, err)
	}
	return string(printed), code
}

// cleanEnvironment is the environment a script under test runs in: a temporary
// home, the fakes in front of the real PATH, and nothing else that could reach
// out to the network or to the machine's own settings.
func cleanEnvironment(home string, fakes string, extra ...string) []string {
	environment := []string{
		"HOME=" + home,
		"PATH=" + fakes + ":/usr/local/go/bin:/usr/bin:/bin",
		"SHELL=/bin/sh",
		"LANG=C",
	}
	return append(environment, extra...)
}

// writeExecutable writes one file and makes it runnable, which is how every fake
// program in these tests is made.
func writeExecutable(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("cannot make the folder for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("cannot write the program %s: %v", path, err)
	}
}

// writeFile writes one plain file, making the folders above it first.
func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("cannot make the folder for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("cannot write the file %s: %v", path, err)
	}
}

// readFile reads one file the script under test wrote, and fails the test with
// the path in the message when it is not there.
func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read %s, which the script should have written: %v", path, err)
	}
	return string(content)
}

// copyScript copies one script out of the checkout into the folder the test runs
// it from, so that a test can never write over the real one, and returns where
// it put it.
func copyScript(t *testing.T, into string, relative string) string {
	t.Helper()
	source := filepath.Join(repositoryRoot(t), filepath.FromSlash(relative))
	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("cannot read the script %s: %v", source, err)
	}
	destination := filepath.Join(into, filepath.Base(relative))
	writeExecutable(t, destination, string(content))
	return destination
}

// recordingProgram is the body of a fake program: it writes the name it was
// called by and every argument it was given onto its own line of a log file, so
// that a test can assert on what the script asked the machine to do.
func recordingProgram(name string, log string, exitCode int) string {
	return "#!/bin/sh\n" +
		"printf '%s %s\\n' " + name + " \"$*\" >> " + log + "\n" +
		"exit " + strconv.Itoa(exitCode) + "\n"
}

// namesInArchive lists every path inside a gzipped tar archive, which is how the
// tests check that a release archive holds what it promises.
func namesInArchive(t *testing.T, path string) []string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("cannot open the archive %s: %v", path, err)
	}
	defer func() { _ = file.Close() }()

	unzipped, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("the archive %s is not gzipped: %v", path, err)
	}
	defer func() { _ = unzipped.Close() }()

	names := []string{}
	reader := tar.NewReader(unzipped)
	for len(names) < maxArchiveEntries {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("cannot read the archive %s: %v", path, err)
		}
		names = append(names, strings.TrimSuffix(header.Name, "/"))
	}
	return names
}

// linksInArchive lists every entry that is not a plain file or a folder, which
// is what internal/update refuses when it unpacks a release.
func linksInArchive(t *testing.T, path string) []string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("cannot open the archive %s: %v", path, err)
	}
	defer func() { _ = file.Close() }()

	unzipped, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("the archive %s is not gzipped: %v", path, err)
	}
	defer func() { _ = unzipped.Close() }()

	links := []string{}
	reader := tar.NewReader(unzipped)
	for read := 0; read < maxArchiveEntries; read++ {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("cannot read the archive %s: %v", path, err)
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir {
			links = append(links, header.Name)
		}
	}
	return links
}

// maxArchiveEntries caps how much of an archive a test will read, because every
// loop has a limit. A release archive holds a few tens of thousands of files at
// most, nearly all of them the two workers' dependencies.
const maxArchiveEntries = 200000

// containsPathEnding says whether any listed path ends with the piece given,
// which is how a test names one file inside an archive without writing the whole
// version-stamped folder name in front of it.
func containsPathEnding(names []string, ending string) bool {
	for _, name := range names {
		if strings.HasSuffix(name, ending) {
			return true
		}
	}
	return false
}

// requirePrinted fails the test unless the script printed the words given, and
// shows everything it did print, because a message is part of what the installer
// promises its user.
func requirePrinted(t *testing.T, printed string, want string, why string) {
	t.Helper()
	if !strings.Contains(printed, want) {
		t.Errorf("%s: nothing said %q. The script printed:\n%s", why, want, printed)
	}
}
