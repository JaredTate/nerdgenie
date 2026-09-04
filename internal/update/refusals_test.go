package update

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// anEmptyDatabase is a SQLite file with nothing in it, which is what every one
// of these refusals is tried against.
func anEmptyDatabase(t *testing.T) *sql.DB {
	t.Helper()
	database, err := openDatabase(filepath.Join(t.TempDir(), "empty.db"))
	if err != nil {
		t.Fatalf("opening an empty database failed: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func TestRecordingAMigrationInAFileWithNoSchemaVersionTableIsRefused(t *testing.T) {
	database := anEmptyDatabase(t)

	err := recordMigration(context.Background(), database, Migration{To: 2, Name: "one that cannot be written down"}, "0.8.0", theTestMoment)

	if err == nil {
		t.Fatalf("a migration was written down in a file that is not a Nerd Genie database")
	}
	if !strings.Contains(err.Error(), "schema version") {
		t.Errorf("the refusal does not say which write failed: %v", err)
	}
}

func TestAnEmptySchemaVersionTableIsRefused(t *testing.T) {
	database := anEmptyDatabase(t)
	if _, err := database.Exec("CREATE TABLE schema_version (version INTEGER NOT NULL PRIMARY KEY)"); err != nil {
		t.Fatalf("making an empty version table failed: %v", err)
	}

	if _, err := readSchemaVersion(context.Background(), database); err == nil {
		t.Errorf("a file with an empty schema version table was read as a Nerd Genie database")
	}
}

func TestAReleasesFolderThatIsAFileIsRefused(t *testing.T) {
	home := contract.NewHome(t.TempDir())
	if err := os.WriteFile(home.ReleasesFolder(), []byte("not a folder"), contract.DataFileMode); err != nil {
		t.Fatalf("writing a file where the releases folder goes failed: %v", err)
	}
	manifest := Manifest{
		Version:       "0.7.0",
		Architectures: []string{runtime.GOARCH},
		Checksums:     map[string]string{ArchiveName("0.7.0", runtime.GOARCH): strings.Repeat("a", ChecksumDigits)},
	}

	if _, err := installRelease(context.Background(), home, Source{Address: t.TempDir()}, manifest, runtime.GOARCH); err == nil {
		t.Errorf("a release was installed into a releases folder that is a file")
	}
	if _, err := installedVersions(home); err == nil {
		t.Errorf("a releases folder that is a file was read as a list of versions")
	}
	if err := pruneReleases(home, KeptReleases); err == nil {
		t.Errorf("a releases folder that is a file was pruned")
	}
}

func TestAnAddressThatIsNotAnAddressAtAllIsRefused(t *testing.T) {
	source := Source{Address: "https://example.invalid/%zz"}

	if _, err := source.Fetch(context.Background(), ManifestName, &bytes.Buffer{}); err == nil {
		t.Errorf("an address that cannot be spelt as a web address was read")
	}
}

func TestALinkThatCannotBeMadeIsRefused(t *testing.T) {
	home := contract.NewHome(t.TempDir())
	if err := os.MkdirAll(home.ReleasesFolder(), contract.HomeFolderMode); err != nil {
		t.Fatalf("making the releases folder failed: %v", err)
	}
	binary := filepath.Join(t.TempDir(), BinaryName)
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("writing the program failed: %v", err)
	}
	if err := os.Chmod(home.ReleasesFolder(), 0o500); err != nil {
		t.Fatalf("making the releases folder read only failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(home.ReleasesFolder(), contract.HomeFolderMode) })

	if err := linkRelease(home, binary); err == nil {
		t.Errorf("a link was made in a folder that cannot be written to")
	}
	if err := linkRelease(home, filepath.Join(t.TempDir(), "not-there")); err == nil {
		t.Errorf("the link was pointed at a program that is not there")
	}
}

func TestAProgramOutsideAReleaseFolderHasNoVersion(t *testing.T) {
	if version := versionOf(""); version != "" {
		t.Errorf("a program that is not there has the version %q", version)
	}
	if version := versionOf(filepath.Join("releases", "0.7.0", BinaryName)); version != "0.7.0" {
		t.Errorf("the program in the 0.7.0 folder has the version %q", version)
	}
}
