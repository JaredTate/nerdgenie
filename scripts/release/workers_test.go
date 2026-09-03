package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// "make build" promises the binary in bin/coeus and the worker bundles in
// bin/workers/<name>/, and CLAUDE.md and docs/WORK_PLAN.md both say so, so that
// is what these tests hold the script to. A release lays the workers out the same
// way, which is why one path finds a worker in a checkout and in an install.

// runWorkers runs scripts/release/workers.sh over a fixture checkout with a fake
// npm on the front of the PATH, so that no test reaches the npm registry.
func runWorkers(t *testing.T, root string) string {
	t.Helper()
	environment := cleanEnvironment(root, fakeNpm(t, root))
	script := filepath.Join(root, "scripts", "release", "workers.sh")
	printed, code := runScript(t, script, root, environment)
	if code != 0 {
		t.Fatalf("workers.sh exited %d, want 0. It printed:\n%s", code, printed)
	}
	return printed
}

func TestTheWorkerBundlesLandWhereMakeBuildPromises(t *testing.T) {
	root := fixtureCheckout(t)

	printed := runWorkers(t, root)

	for _, worker := range []string{"browser", "desktop"} {
		bundle := filepath.Join(root, "bin", "workers", worker, "main.js")
		if _, err := os.Stat(bundle); err != nil {
			t.Errorf("%s is not there after workers.sh: %v\nIt printed:\n%s", bundle, err, printed)
		}
		link := filepath.Join(root, "bin", "workers", worker, "node_modules")
		pointsAt, err := os.Readlink(link)
		if err != nil {
			t.Errorf("%s is not a link to the worker's dependencies: %v", link, err)
			continue
		}
		if pointsAt != filepath.Join(root, "worker", worker, "node_modules") {
			t.Errorf("%s points at %s, want the worker's own node_modules", link, pointsAt)
		}
	}
}

func TestABundleThatIsAlreadyBuiltIsNotBuiltAgain(t *testing.T) {
	root := fixtureCheckout(t)

	printed := runWorkers(t, root)

	requirePrinted(t, printed, "already up to date", "a bundle newer than its sources is left alone")
	if asked := npmWasAsked(t, root); strings.Contains(asked, "run build") {
		t.Errorf("npm was asked to build a bundle that was already up to date:\n%s", asked)
	}
}

func TestABundleOlderThanItsSourceIsBuiltAgain(t *testing.T) {
	root := fixtureCheckout(t)
	// One source file touched after the bundle was written is the whole reason to
	// build again, so the test says exactly that rather than sleeping.
	later := time.Now().Add(time.Hour)
	source := filepath.Join(root, "worker", "browser", "src", "main.ts")
	if err := os.Chtimes(source, later, later); err != nil {
		t.Fatalf("cannot make %s look newer than the bundle: %v", source, err)
	}

	runWorkers(t, root)

	asked := npmWasAsked(t, root)
	if !strings.Contains(asked, "run build") {
		t.Errorf("a bundle older than its source was not built again. npm was asked:\n%s", asked)
	}
}
