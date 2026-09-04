package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/update"
)

// aReleaseToInstall writes a release into a folder: an archive holding a program
// that comes up and answers, the sums file, and the manifest naming both.
func aReleaseToInstall(t *testing.T, folder string, version string) {
	t.Helper()
	program := "#!/bin/sh\nexit 0\n"
	packed := bytes.Buffer{}
	compressor := gzip.NewWriter(&packed)
	archive := tar.NewWriter(compressor)
	header := &tar.Header{Name: update.BinaryName, Mode: 0o755, Size: int64(len(program)), Typeflag: tar.TypeReg}
	if err := archive.WriteHeader(header); err != nil {
		t.Fatalf("writing the archive header failed: %v", err)
	}
	if _, err := archive.Write([]byte(program)); err != nil {
		t.Fatalf("writing the program into the archive failed: %v", err)
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("closing the archive failed: %v", err)
	}
	if err := compressor.Close(); err != nil {
		t.Fatalf("closing the compressor failed: %v", err)
	}

	name := update.ArchiveName(version, runtime.GOARCH)
	sum := sha256.Sum256(packed.Bytes())
	written, err := json.Marshal(update.Manifest{
		Version:       version,
		Date:          "2026-09-02",
		Architectures: []string{runtime.GOARCH},
		Checksums:     map[string]string{name: hex.EncodeToString(sum[:])},
	})
	if err != nil {
		t.Fatalf("writing the manifest failed: %v", err)
	}
	for path, body := range map[string][]byte{
		filepath.Join(folder, name):                 packed.Bytes(),
		filepath.Join(folder, update.ManifestName):  written,
		filepath.Join(folder, update.ChecksumsName): []byte(hex.EncodeToString(sum[:]) + "  " + name + "\n"),
	} {
		if err := os.WriteFile(path, body, contract.DataFileMode); err != nil {
			t.Fatalf("writing %s failed: %v", path, err)
		}
	}
}

// aServiceThatAnswers gives the test its own home folder under a short path, a
// systemctl of its own that does nothing, and an agent on the local socket that
// answers the readiness check, so that no test here touches the real service
// manager or waits out the readiness deadline.
func aServiceThatAnswers(t *testing.T) contract.Home {
	t.Helper()
	folder, err := os.MkdirTemp("", "nerdgenie-cmd")
	if err != nil {
		t.Fatalf("making a folder for the home failed: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(folder) })
	t.Setenv("HOME", folder)

	home := contract.NewHome(filepath.Join(folder, contract.HomeFolderName))
	for _, one := range home.Folders() {
		if err := os.MkdirAll(one, contract.HomeFolderMode); err != nil {
			t.Fatalf("making the folder %s failed: %v", one, err)
		}
	}

	onThePath := t.TempDir()
	script := "#!/bin/sh\necho \"$*\" >> " + filepath.Join(onThePath, "told.txt") + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(onThePath, "systemctl"), []byte(script), 0o755); err != nil {
		t.Fatalf("writing the fake systemctl failed: %v", err)
	}
	t.Setenv("PATH", onThePath)
	answerTheReadinessCheck(t, home)
	return home
}

// answerTheReadinessCheck stands in for a running agent: it answers anything
// asked on the local socket, which is what "nerdgenie update" waits for after it has
// started the service.
func answerTheReadinessCheck(t *testing.T, home contract.Home) {
	t.Helper()
	listener, err := net.Listen("unix", home.SocketFile())
	if err != nil {
		t.Fatalf("listening on the agent's socket at %s failed: %v", home.SocketFile(), err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			_ = contract.EncodeSocketEnvelope(connection, contract.SocketEnvelope{
				Type: contract.SocketReply, Text: "ready",
			})
			_ = connection.Close()
		}
	}()
}

// aMachineReadyToUpdate gives the test a home with one version installed, a
// systemctl of its own that starts whatever the link points at, and a folder
// holding the release on offer.
func aMachineReadyToUpdate(t *testing.T) (contract.Home, string) {
	t.Helper()
	home := aServiceThatAnswers(t)
	installed := home.ReleaseFolder("0.0.1")
	if err := os.MkdirAll(installed, contract.HomeFolderMode); err != nil {
		t.Fatalf("making the installed release folder failed: %v", err)
	}
	binary := filepath.Join(installed, update.BinaryName)
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("writing the installed program failed: %v", err)
	}
	if err := os.Symlink(binary, home.CurrentReleaseLink()); err != nil {
		t.Fatalf("pointing the current link at the installed program failed: %v", err)
	}
	address := t.TempDir()
	aReleaseToInstall(t, address, "9.9.9")
	return home, address
}

func TestUpdateSubcommandChecksWithoutChangingAnything(t *testing.T) {
	home, address := aMachineReadyToUpdate(t)
	var output, problems bytes.Buffer

	code := updateSubcommand.run([]string{"--check", "--from", address}, &output, &problems)

	if code != contract.ExitOK {
		t.Fatalf("nerdgenie update --check left with %d: %s", code, problems.String())
	}
	if !strings.Contains(output.String(), "9.9.9") {
		t.Errorf("the check does not say what is on offer:\n%s", output.String())
	}
	if _, err := os.Stat(home.ReleaseFolder("9.9.9")); !os.IsNotExist(err) {
		t.Errorf("a check installed something")
	}
}

func TestUpdateSubcommandInstallsTheVersionOnOffer(t *testing.T) {
	home, address := aMachineReadyToUpdate(t)
	var output, problems bytes.Buffer

	code := updateSubcommand.run([]string{"--from", address}, &output, &problems)

	if code != contract.ExitOK {
		t.Fatalf("nerdgenie update left with %d: %s", code, problems.String())
	}
	where, err := os.Readlink(home.CurrentReleaseLink())
	if err != nil {
		t.Fatalf("reading the current link failed: %v", err)
	}
	if where != filepath.Join(home.ReleaseFolder("9.9.9"), update.BinaryName) {
		t.Errorf("the current link points at %s rather than the new release", where)
	}
	if !strings.Contains(output.String(), "9.9.9") {
		t.Errorf("the update does not say which version is running now:\n%s", output.String())
	}
}

func TestUpdateSubcommandRefusesAVersionTheAddressDoesNotOffer(t *testing.T) {
	_, address := aMachineReadyToUpdate(t)
	var output, problems bytes.Buffer

	code := updateSubcommand.run([]string{"--from", address, "--to", "0.0.1"}, &output, &problems)

	if code != contract.ExitFailure {
		t.Errorf("nerdgenie update --to a version the address does not offer left with %d rather than %d", code, contract.ExitFailure)
	}
	if !strings.Contains(problems.String(), "0.0.1") {
		t.Errorf("the refusal does not say which version was asked for:\n%s", problems.String())
	}
}

func TestUpdateSubcommandBringsTheDatabaseForward(t *testing.T) {
	testkit.NewTempHome(t)
	var output, problems bytes.Buffer

	code := updateSubcommand.run([]string{"--migrate"}, &output, &problems)

	if code != contract.ExitOK {
		t.Fatalf("nerdgenie update --migrate left with %d on a home with no database: %s", code, problems.String())
	}
	if !strings.Contains(output.String(), "database") {
		t.Errorf("the migration step says nothing about the database:\n%s", output.String())
	}
}

func TestUpdateSubcommandRefusesFlagsThatContradictEachOther(t *testing.T) {
	cases := [][]string{
		{"--check", "--rollback"},
		{"--rollback", "--to", "0.7.0"},
		{"--migrate", "--check"},
		{"--nothing-like-this"},
		{"a-word-it-does-not-understand"},
	}

	for _, arguments := range cases {
		t.Run(strings.Join(arguments, " "), func(t *testing.T) {
			aMachineReadyToUpdate(t)
			var output, problems bytes.Buffer

			if code := updateSubcommand.run(arguments, &output, &problems); code != contract.ExitUsage {
				t.Errorf("nerdgenie update %v left with %d rather than %d", arguments, code, contract.ExitUsage)
			}
		})
	}
}

func TestUpdateSubcommandRollsBackToThePreviousRelease(t *testing.T) {
	home, address := aMachineReadyToUpdate(t)
	var output, problems bytes.Buffer
	if code := updateSubcommand.run([]string{"--from", address}, &output, &problems); code != contract.ExitOK {
		t.Fatalf("the update before the rollback failed: %s", problems.String())
	}

	code := updateSubcommand.run([]string{"--rollback"}, &output, &problems)

	if code != contract.ExitOK {
		t.Fatalf("nerdgenie update --rollback left with %d: %s", code, problems.String())
	}
	where, err := os.Readlink(home.CurrentReleaseLink())
	if err != nil {
		t.Fatalf("reading the current link failed: %v", err)
	}
	if where != filepath.Join(home.ReleaseFolder("0.0.1"), update.BinaryName) {
		t.Errorf("the current link points at %s rather than back at the version before", where)
	}
}

func TestUpdateSubcommandSaysWhenTheAddressHasNothingOnIt(t *testing.T) {
	aMachineReadyToUpdate(t)
	var output, problems bytes.Buffer

	code := updateSubcommand.run([]string{"--from", filepath.Join(t.TempDir(), "nothing")}, &output, &problems)

	if code != contract.ExitFailure {
		t.Errorf("nerdgenie update against an address with nothing on it left with %d rather than %d", code, contract.ExitFailure)
	}
	if problems.String() == "" {
		t.Errorf("nothing was printed about why the update did not happen")
	}
}
