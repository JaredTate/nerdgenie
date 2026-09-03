// The check on the database after an unclean exit follows the recovery path in
// Hermes at ~/Code/hermes-agent/gateway/session_db_recovery.py and the integrity
// check its lifecycle ledger runs at ~/Code/hermes-agent/gateway/
// lifecycle_ledger.py. The lesson taken from both is that the moment to ask
// whether the file is still a database is before anything else opens it, and
// that a file which fails the check is moved aside rather than repaired in
// place, so that the evidence survives and the agent comes back on a good file.

package reliability

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	// The pure-Go SQLite driver, registered under the name "sqlite", which is the
	// same one the event log opens the file with.
	_ "modernc.org/sqlite"
)

// maxCheckLines is how many complaints the quick check is read out of before
// the rest is left unread, because a file that is broken in a hundred places is
// as broken as a file that is broken in five.
const maxCheckLines = 5

// databaseWait is the option every connection here is opened with: wait a few
// seconds for another connection's write lock rather than failing at once, so
// that a check run while the agent is writing is not a false alarm.
const databaseWait = "?_pragma=busy_timeout(5000)"

// CheckDatabase runs SQLite's own quick check over the file and returns an error
// saying what is wrong when the file is not a database Coeus can use. A file
// that is not there yet is not a fault, because a fresh home has no database
// until the first event is written.
func CheckDatabase(ctx context.Context, path string) error {
	if !databaseIsThere(path) {
		return nil
	}
	handle, err := sql.Open("sqlite", path+databaseWait)
	if err != nil {
		return fmt.Errorf("the database %s could not be opened to check it: %w", path, err)
	}
	defer func() { _ = handle.Close() }()

	rows, err := handle.QueryContext(ctx, "PRAGMA quick_check")
	if err != nil {
		return fmt.Errorf("the database %s did not answer SQLite's own quick check, so it is damaged: %w", path, err)
	}
	defer func() { _ = rows.Close() }()

	complaints := []string{}
	for len(complaints) < maxCheckLines && rows.Next() {
		line := ""
		if err := rows.Scan(&line); err != nil {
			return fmt.Errorf("the answer to the quick check on %s could not be read, so the file is damaged: %w", path, err)
		}
		if line != "ok" {
			complaints = append(complaints, line)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("the quick check on %s stopped part way, so the file is damaged: %w", path, err)
	}
	if len(complaints) > 0 {
		return fmt.Errorf("the database %s failed SQLite's quick check, so it cannot be trusted: %s", path, strings.Join(complaints, "; "))
	}
	return nil
}

// copyDatabase writes a copy of the database that is complete on its own, using
// SQLite's own VACUUM INTO, which takes a read lock and writes a file holding
// everything the write-ahead file has as well. Copying the file with a plain
// read while the agent is running would catch it mid-write.
func copyDatabase(ctx context.Context, path string, into string) error {
	handle, err := sql.Open("sqlite", path+databaseWait)
	if err != nil {
		return fmt.Errorf("the database %s could not be opened to copy it: %w", path, err)
	}
	defer func() { _ = handle.Close() }()

	if _, err := handle.ExecContext(ctx, "VACUUM INTO ?", into); err != nil {
		return fmt.Errorf("a copy of the database %s could not be written to %s: %w", path, into, err)
	}
	return nil
}
