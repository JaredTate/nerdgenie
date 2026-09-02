// Package sandbox runs a command inside bwrap with a Landlock ruleset and a
// seccomp filter.
//
// The fence has two halves. Outside, this package builds a bwrap command line
// that gives the command a new user, process, message-queue, and hostname
// namespace, a fresh /proc, /dev, and /tmp, the system folders bound read-only,
// and the configured sandbox roots bound read-write at their own paths. The
// network is left alone, because the agent's tools need it. Nothing else is
// bound, so the agent's own home folder, the vault, the browser profile, and the
// user's SSH keys are not there to be read.
//
// Inside, bwrap cannot apply Landlock, so it starts the coeus binary again as a
// hidden subcommand, sandbox-entry. That helper locks its operating-system
// thread, sets the no-new-privileges flag, builds a Landlock ruleset that allows
// reading and running the system folders and full access to the roots, installs
// a seccomp filter that answers a short list of system calls no tool needs with
// "operation not permitted", and then becomes the command it was asked to run.
// Both restrictions survive that change of program, so the command starts
// already fenced in.
//
// Every run has bounds: the number of roots, the number of arguments, the number
// of environment entries, the time the command may take, and the bytes kept from
// each of its two output streams.
package sandbox
