// The recovery that has to happen before anything opens the database follows
// the recovery path in Hermes at ~/Code/hermes-agent/gateway/
// session_db_recovery.py, whose lesson is that the moment to ask whether the
// file is still a database is before any connection to it exists. A handle
// taken before the check still points at the file the check renames, so every
// row written through it lands in the file that was moved aside. That is why
// this is a call of its own that hands back the path to open, and why the guard
// refuses to start until it has run.

package reliability

import (
	"context"
	"errors"
	"fmt"

	"github.com/JaredTate/coeus/internal/contract"
)

// RecoverySettings is what the recovery needs, and it is deliberately less than
// the whole guard needs: the recovery runs before the event log is open, so it
// cannot be given one.
type RecoverySettings struct {
	// Home is the folder the agent keeps everything in.
	Home contract.Home
	// Clock is where the recovery reads the time, for the name of a database it
	// moves aside.
	Clock contract.Clock
	// BackupFolder is where the archives are, and is the home's backups folder
	// when it is empty. The configuration's backup_path is passed here.
	BackupFolder string
}

// recovery is what the check found before the database was opened. It is
// written into the sentinel, so that Guard.Start can tell the user about it
// once there is a channel to tell them on.
type recovery struct {
	// FollowedAnUncleanExit says the life before this one left the sentinel
	// behind, which means no exit path ran.
	FollowedAnUncleanExit bool `json:"followedAnUncleanExit"`
	// DatabaseMovedTo is where a database that failed its check was moved.
	DatabaseMovedTo string `json:"databaseMovedTo,omitempty"`
	// WhatWasWrong is what the check said about the file it moved aside.
	WhatWasWrong string `json:"whatWasWrong,omitempty"`
	// RestoredFrom is the archive the database was put back from.
	RestoredFrom string `json:"restoredFrom,omitempty"`
	// CouldNotRestore is why no archive could be put back, when none could.
	CouldNotRestore string `json:"couldNotRestore,omitempty"`
}

// PrepareDatabase does everything that has to happen before any package opens
// the database, and hands back the path to open. It sees whether the last life
// of the program ended uncleanly, and when it did it runs SQLite's own check
// over the file, moves a damaged one aside, and puts the newest archive back in
// its place. Take the path from here and nowhere else: a handle opened before
// this call still points at the file it moved aside, and every event written
// through that handle is lost. Guard.Start refuses to run until this has.
func PrepareDatabase(ctx context.Context, settings RecoverySettings) (string, error) {
	if settings.Clock == nil {
		return "", errors.New("the database recovery needs a clock, so pass clock.System() or the one the test controls")
	}
	sentinel := newSentinel(settings.Home, settings.Clock)
	databaseFile := settings.Home.DatabaseFile()
	found := recovery{FollowedAnUncleanExit: sentinel.leftBehind()}

	if found.FollowedAnUncleanExit {
		if err := recoverDatabase(ctx, settings, databaseFile, &found); err != nil {
			return "", err
		}
	}
	// The sentinel is written last, so that a program killed in the middle of a
	// recovery is still an unclean exit to the life after it and the recovery
	// runs again.
	if err := sentinel.startThisLife(found); err != nil {
		return "", err
	}
	return databaseFile, nil
}

// recoverDatabase checks the database and, when it is damaged, moves it aside
// and puts the newest backup in its place. A recovery that cannot find or read
// an archive is written down rather than raised, because an agent that comes up
// with an empty database can still be talked to and one that refuses to start
// cannot.
func recoverDatabase(ctx context.Context, settings RecoverySettings, databaseFile string, found *recovery) error {
	broken := CheckDatabase(ctx, databaseFile)
	if broken == nil {
		return nil
	}
	found.WhatWasWrong = broken.Error()

	movedTo, err := MoveDatabaseAside(databaseFile, settings.Clock.Now())
	if err != nil {
		return err
	}
	found.DatabaseMovedTo = movedTo

	archive, err := LatestArchive(backupFolderOf(settings))
	if err == nil {
		err = Restore(ctx, RestoreSettings{
			Home:            settings.Home,
			Archive:         archive,
			OnlyTheDatabase: true,
			Force:           true,
		})
	}
	if err != nil {
		found.CouldNotRestore = err.Error()
		return nil
	}
	found.RestoredFrom = archive
	return nil
}

// backupFolderOf is where the archives live: what the configuration said, or
// the backups folder in the home.
func backupFolderOf(settings RecoverySettings) string {
	if settings.BackupFolder != "" {
		return settings.BackupFolder
	}
	return settings.Home.BackupsFolder()
}

// theRecoveryMessage is what the user is told about a database that was moved
// aside, and is empty when nothing was.
func theRecoveryMessage(found recovery) string {
	switch {
	case found.DatabaseMovedTo == "":
		return ""
	case found.CouldNotRestore != "":
		return fmt.Sprintf(
			"Coeus found its database damaged (%s) and moved it to %s. It could not put a backup back (%s), so it is starting with an empty one. Everything it was doing is in the file it moved aside.",
			found.WhatWasWrong, found.DatabaseMovedTo, found.CouldNotRestore)
	default:
		return fmt.Sprintf(
			"Coeus found its database damaged (%s), moved it to %s, and put back the backup %s. Anything it learned after that backup is only in the file it moved aside.",
			found.WhatWasWrong, found.DatabaseMovedTo, found.RestoredFrom)
	}
}
