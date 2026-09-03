package reliability

import (
	"context"
	"errors"
	"fmt"

	"github.com/JaredTate/coeus/internal/contract"
)

// Settings is everything the guard needs from the program around it. serve.go
// fills it in once and hands the guard to the loop.
type Settings struct {
	// Home is the folder the agent keeps everything in.
	Home contract.Home
	// Clock is where every part of the guard reads the time.
	Clock contract.Clock
	// Store is the event log, which is where the delivery ledger writes.
	Store contract.Store
	// Caps are the limits the two deadlines are made from.
	Caps contract.Caps
	// BackupFolder is where the archives are, and is the home's backups folder
	// when it is empty. The configuration's backup_path is passed here.
	BackupFolder string
	// Send delivers one message to the user. An empty channel means the channel
	// the user is normally talked to on.
	Send func(ctx context.Context, channel string, text string) error
}

// Startup is what the guard found when the program started, so that serve.go
// can log it and the terminal can show it.
type Startup struct {
	// UncleanExit says the last life of the program ended without an exit path.
	UncleanExit bool
	// DatabaseMovedTo is where a database that failed its check was moved, and
	// is empty when the database was fine.
	DatabaseMovedTo string
	// RestoredFrom is the archive the database was put back from, and is empty
	// when nothing had to be put back.
	RestoredFrom string
	// BreakerTripped says the agent will serve the user but start no task.
	BreakerTripped bool
	// RepliesSentAgain is how many replies from the last life were delivered
	// again with the duplicate marker.
	RepliesSentAgain int
}

// Guard is the whole reliability layer: the breaker, the lease registry, the
// delivery ledger, the sentinel, the drain marker, the deadlines, and the
// watchdog feed, wired together and started in the right order.
type Guard struct {
	settings Settings
	breaker  *Breaker
	leases   *Leases
	ledger   *Ledger
	sentinel *Sentinel
	drain    *Drain
	watchdog *Watchdog
}

// New builds the guard serve.go wires around the loop.
func New(settings Settings) (*Guard, error) {
	if settings.Clock == nil {
		return nil, errors.New("the reliability guard needs a clock, so pass clock.System() or the one the test controls")
	}
	if settings.Store == nil {
		return nil, errors.New("the reliability guard needs the event log, because a reply is written down before it is sent")
	}
	if settings.Send == nil {
		return nil, errors.New("the reliability guard needs a way of talking to the user, so pass the function that sends one message")
	}
	watchdog, err := NewWatchdog(settings.Clock)
	if err != nil {
		return nil, err
	}
	return &Guard{
		settings: settings,
		breaker:  NewBreaker(settings.Home, settings.Clock),
		leases:   NewLeases(settings.Clock),
		ledger:   NewLedger(settings.Store, settings.Clock),
		sentinel: NewSentinel(settings.Home, settings.Clock),
		drain:    NewDrain(settings.Home, settings.Clock),
		watchdog: watchdog,
	}, nil
}

// Start does the whole startup sequence in order: see whether the last exit was
// unclean, check the database before anything opens it, count the crash against
// the breaker, tell the user if it has tripped, and send again the replies that
// were written down but never delivered.
func (guard *Guard) Start(ctx context.Context) (Startup, error) {
	found := Startup{}
	unclean, err := guard.sentinel.Start()
	found.UncleanExit = unclean
	if err != nil {
		return found, err
	}

	if unclean {
		if err := guard.recoverDatabase(ctx, &found); err != nil {
			return found, err
		}
		if found.BreakerTripped, err = guard.breaker.RecordUncleanStart(); err != nil {
			return found, err
		}
	} else if found.BreakerTripped, err = guard.breaker.Tripped(); err != nil {
		return found, err
	}

	if err := guard.announceTheBreaker(ctx); err != nil {
		return found, err
	}
	found.RepliesSentAgain, err = guard.ledger.Resend(ctx, guard.settings.Send)
	return found, err
}

// Stop is the clean exit: the sentinel goes, so the next start knows nothing
// went wrong, and the crash loop is forgotten.
func (guard *Guard) Stop() error {
	return errors.Join(guard.sentinel.MarkExited(), guard.breaker.Clear())
}

// MayStartTask says whether the loop may pick up new work. It is false while
// the breaker is tripped and while a drain is on.
func (guard *Guard) MayStartTask() bool {
	return guard.WhyNoNewTask() == ""
}

// Healthy says whether the agent is in a crash loop, which is what a screen's
// health mark shows. A broken breaker file leaves the answer healthy, because a
// breaker nobody can read must never make a working agent look broken.
func (guard *Guard) Healthy() bool {
	tripped, err := guard.breaker.Tripped()
	return err != nil || !tripped
}

// AcquireTurn takes the turn lease for a session, waiting for a turn that is
// already running and refusing this one when the wait runs out.
func (guard *Guard) AcquireTurn(ctx context.Context, session string) (*Lease, error) {
	return guard.leases.Acquire(ctx, session)
}

// TurnDeadline is the limit on one whole turn, starting now.
func (guard *Guard) TurnDeadline() Deadline {
	return TurnDeadline(guard.settings.Clock, guard.settings.Caps)
}

// ToolDeadline is the limit on one tool call, starting now.
func (guard *Guard) ToolDeadline() Deadline {
	return ToolDeadline(guard.settings.Clock, guard.settings.Caps)
}

// Ledger is where the loop writes a reply down before it sends it.
func (guard *Guard) Ledger() *Ledger { return guard.ledger }

// Drain is the marker the updater writes to stop new work.
func (guard *Guard) Drain() *Drain { return guard.drain }

// Ready tells the service manager that the program is up and answering, which
// serve.go calls when the readiness check first answers.
func (guard *Guard) Ready() error { return guard.watchdog.Ready() }

// FeedWatchdog keeps telling the service manager that the program is alive
// until the context ends. It goes on feeding while the breaker is tripped,
// because a tripped breaker means the agent answers the user and starts no
// task: a feed that stopped would have systemd kill the program, the kill would
// leave the sentinel behind, the next start would count as one more crash, and
// the task that was killing the program would be picked up again.
func (guard *Guard) FeedWatchdog(ctx context.Context) error {
	return guard.watchdog.Feed(ctx)
}

// Backup writes one archive of the database, the vault, and the browser
// profile, which is what the nightly timer runs.
func (guard *Guard) Backup(ctx context.Context) (string, error) {
	return Backup(ctx, BackupSettings{
		Home:   guard.settings.Home,
		Clock:  guard.settings.Clock,
		Folder: guard.backupFolder(),
	})
}

// announceTheBreaker sends the one message the user gets about a crash loop.
func (guard *Guard) announceTheBreaker(ctx context.Context) error {
	said, err := guard.breaker.Announce()
	if err != nil || said == "" {
		return err
	}
	return guard.settings.Send(ctx, "", said)
}

// recoverDatabase checks the database after an unclean exit and, when it is
// damaged, moves it aside and puts back the newest backup. A recovery that
// cannot find or read a backup is told to the user rather than raised, because
// an agent that comes up with an empty database can still be talked to and an
// agent that refuses to start cannot.
func (guard *Guard) recoverDatabase(ctx context.Context, found *Startup) error {
	path := guard.settings.Home.DatabaseFile()
	broken := CheckDatabase(ctx, path)
	if broken == nil {
		return nil
	}
	movedTo, err := MoveDatabaseAside(path, guard.settings.Clock.Now())
	if err != nil {
		return err
	}
	found.DatabaseMovedTo = movedTo

	archive, err := LatestArchive(guard.backupFolder())
	if err == nil {
		err = Restore(ctx, RestoreSettings{
			Home:            guard.settings.Home,
			Archive:         archive,
			OnlyTheDatabase: true,
			Force:           true,
		})
	}
	if err != nil {
		return guard.settings.Send(ctx, "", fmt.Sprintf(
			"Coeus found its database damaged (%v) and moved it to %s. It could not put a backup back (%v), so it is starting with an empty one. Everything it was doing is in the file it moved aside.",
			broken, movedTo, err))
	}
	found.RestoredFrom = archive
	return guard.settings.Send(ctx, "", fmt.Sprintf(
		"Coeus found its database damaged (%v), moved it to %s, and put back the backup %s. Anything it learned after that backup is only in the file it moved aside.",
		broken, movedTo, archive))
}

// backupFolder is where the archives live: what the configuration said, or the
// backups folder in the home.
func (guard *Guard) backupFolder() string {
	if guard.settings.BackupFolder != "" {
		return guard.settings.BackupFolder
	}
	return guard.settings.Home.BackupsFolder()
}
