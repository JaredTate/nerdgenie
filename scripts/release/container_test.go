//go:build integration

package release

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The whole point of the installer is that it works on a machine nobody has
// touched, which is a thing no fake can prove. These tests run it inside a clean
// Ubuntu container and a clean Debian container, with every init answer given as
// a flag, and end with a passing "coeus doctor". They need a container runtime
// and an archive from "make release"; when either is missing they say so and
// skip, and .github/workflows/install.yml is where they always run.

// containerTimeout bounds one container, because a run that hangs must fail the
// test rather than hold the suite. Installing packages over a slow network is
// the slowest part, and ten minutes is well past it.
const containerTimeout = 10 * time.Minute

// theContainerRuntime returns the name of a container runtime that is installed
// and whose daemon answers, or the empty string when there is none. Docker and
// Podman are both fine; a Docker that is installed but not running is not.
func theContainerRuntime(t *testing.T) string {
	t.Helper()
	for _, runtime := range []string{"docker", "podman"} {
		if _, err := exec.LookPath(runtime); err != nil {
			continue
		}
		ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
		asking := exec.CommandContext(ctx, runtime, "info")
		err := asking.Run()
		stop()
		if err == nil {
			return runtime
		}
		t.Logf("%s is installed but its daemon did not answer, so it cannot be used here", runtime)
	}
	return ""
}

// releaseArchive returns the path of the amd64 archive "make release" wrote, or
// the empty string when the release has not been built here.
func releaseArchive(t *testing.T) string {
	t.Helper()
	folder := filepath.Join(repositoryRoot(t), "dist")
	entries, err := os.ReadDir(folder)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "coeus-") && strings.HasSuffix(name, "-amd64.tar.gz") {
			return filepath.Join(folder, name)
		}
	}
	return ""
}

// insideTheContainer is the shell the container runs: install what a base image
// does not ship, run the installer against the mounted archive with every init
// answer as a flag, and finish with the doctor. Each step is on its own line so
// that a failure names itself.
const insideTheContainer = `set -e
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq --no-install-recommends ca-certificates tar coreutils >/dev/null
echo "--- the installer starts here ---"
sh /coeus/scripts/install.sh --from "/coeus/dist/$ARCHIVE" --no-signal -- \
  --model local --work-folder /root/coeus --signal off --yes
echo "--- the installer finished, now the doctor ---"
/root/.local/bin/coeus doctor
`

// runInstallerInContainer runs the installer inside one image and returns
// everything it printed together with whether it finished cleanly.
func runInstallerInContainer(t *testing.T, runtime string, image string, archive string, distribution string) (string, error) {
	t.Helper()
	ctx, stop := context.WithTimeout(context.Background(), containerTimeout)
	defer stop()

	running := exec.CommandContext(ctx, runtime, "run", "--rm",
		"--env", "ARCHIVE="+filepath.Base(archive),
		"--volume", repositoryRoot(t)+":/coeus:ro",
		image, "sh", "-c", insideTheContainer)
	printed, err := running.CombinedOutput()
	t.Logf("%s in %s printed:\n%s", distribution, image, printed)
	return string(printed), err
}

func TestTheInstallerWorksOnACleanUbuntuAndACleanDebian(t *testing.T) {
	runtime := theContainerRuntime(t)
	if runtime == "" {
		t.Skip("neither Docker nor Podman is running on this machine, so the installer was not tried in a clean " +
			"container here; .github/workflows/install.yml runs this same test on Ubuntu and Debian on every push")
	}
	archive := releaseArchive(t)
	if archive == "" {
		t.Skip("dist/ holds no release archive, so run \"make release\" first and then this test again")
	}
	t.Logf("installing %s with %s", filepath.Base(archive), runtime)

	for _, machine := range []struct {
		distribution string
		image        string
	}{
		{"Ubuntu", "ubuntu:24.04"},
		{"Debian", "debian:12"},
	} {
		t.Run(machine.distribution, func(t *testing.T) {
			printed, err := runInstallerInContainer(t, runtime, machine.image, archive, machine.distribution)
			if err != nil {
				t.Fatalf("the installer did not finish on a clean %s: %v", machine.distribution, err)
			}
			requirePrinted(t, printed, "coeus doctor:", "the doctor ran and printed its report")
			for _, want := range []string{"bubblewrap", "ripgrep", "Chrome", "coeus init"} {
				requirePrinted(t, printed, want, "the installer says what it did on "+machine.distribution)
			}
		})
	}
}
