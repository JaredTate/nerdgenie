// Package reliability holds the mechanisms that keep Coeus serving through a
// crash, a restart, a wedged turn, and a broken database, and the encrypted
// backup that is the last line of defence.
//
// There are eight of them, and each one is small. The crash-loop breaker counts
// unclean starts in a small file under the run folder and, past the limit, keeps
// the program serving while refusing to start a task, so that a task which kills
// the program cannot be replayed forever. The turn lease gives one session one
// turn at a time, so two messages arriving at once cannot write the same record
// twice. The delivery ledger writes every reply into the event log before it is
// sent and marks it delivered after, so a crash between the two is a reply that
// is sent again with a line saying it may be a duplicate, at most three times
// over a day. The lifecycle sentinel is a file that exists only while the
// program is running, so finding it at startup means the last exit was unclean,
// which is when the database is checked and, if it is broken, moved aside and
// replaced from the newest backup. The drain marker tells the loop to finish the
// task it has and take no new one, which is how the updater of wave 6 stops the
// agent without cutting a task in half. The deadline is the one primitive behind
// both the fifteen-minute turn limit and the seven-minute tool limit. The
// watchdog feed tells systemd that the program is alive, and stops telling it
// when the breaker has tripped. And Backup and Restore write and read one
// age-encrypted archive of the database, the vault, and the browser profile.
//
// Guard is the one type serve.go wires: it holds all eight, does the whole
// startup sequence in order, and hands the loop the lease, the deadlines, and
// the ledger. Everything in here reads the time from contract.Clock and never
// from the machine, so every mechanism is tested by moving a fake clock rather
// than by waiting.
package reliability
