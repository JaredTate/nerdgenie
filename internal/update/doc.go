// Package update installs a new version of Coeus beside the running one and
// switches back when it does not come up.
//
// An update is six steps and every one of them can be undone. It reads the
// release manifest from the address it was given, which is a web address in
// normal use and a folder on this machine in a test. It compares the version on
// offer with the version this program was built as. It downloads the archive
// for this machine's architecture, checks it against the checksum the manifest
// gives, and unpacks it into releases/<version> beside the versions already
// there, keeping the newest three. It sets the drain marker, so that the running
// agent finishes the task it has and takes no new one, and stops the service,
// which is the moment that task ends. It points the current link at the new
// binary and starts the service again, and the new version has sixty seconds to
// come up. If it does not, the link goes back to the version that was running,
// the service is started again, and the update says what happened and changed
// nothing else.
//
// The database migrations are the sixth step and they run last, after the new
// version has come up, so that a link switched back always lands on a schema the
// older binary understands. They are forward-only and numbered, each one in a
// transaction, and the whole run happens after a backup: a migration that fails
// brings the agent down, waits until it has really let go of the database, puts
// the backup back, and the update rolls the link back with it. This package
// owns the list of migrations and the schema version that internal/log created
// at one, and CheckSchema is how a binary refuses a database written by a newer
// Coeus while naming the version to use instead.
package update
