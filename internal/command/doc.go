// Package command holds the registry of slash commands and the core commands
// every channel answers, together with the four things a person types before
// there is anything to talk to: init, doctor, install, and uninstall.
//
// There is one command table, so a command means the same thing in the terminal
// and over Signal. Each package exports the commands it owns as
// contract.Command values, and cmd/coeus/serve.go registers them all in one
// place, which is what stops two packages from editing the same registration
// file. The registry splits a typed line into the command's name and its
// arguments, refuses a terminal-only command on any other channel with one
// plain line, and hands the rest to the command itself.
//
// The core commands here are the ones design section 12 names that need nothing
// from another wave-3 package: help, status, model, new, sessions, approve,
// deny, pause, resume, and undo. What they cannot work out for themselves, such
// as the token cost so far or the list of sessions, reaches them through the
// Deps struct of functions that serve.go fills in, so this package never
// imports the loop, the channels, or the terminal screen.
//
// The four subcommands live here too, because they are the same work: init
// makes the home folder, asks at most six questions, writes config.toml and the
// persona files, and prints what to type next; doctor prints what
// config.Doctor found; install writes the systemd user unit and starts the
// service; and uninstall takes it away again. Every one of them accepts every
// answer as a flag, so a container can run them with no keyboard at all.
package command
