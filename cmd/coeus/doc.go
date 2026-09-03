// Package main is the coeus binary: one program that holds the queue, the loop,
// the permissions, the records, the memory, the jobs, and one database file.
//
// This file, the subcommand table in main.go, and serve.go belong to the
// orchestrator, so that no two workers ever edit the same file. A worker who
// adds a subcommand writes a new file in this folder holding one subcommand
// value, and the orchestrator adds that value to the table. A worker who adds a
// slash command exports it as a contract.Command value from its own package, and
// the orchestrator registers it in serve.go.
//
// The table is version, help, init, doctor, serve, tui, install, uninstall,
// signal, askpass, and the sandbox helper, which is hidden from the listing
// because the fence starts it and nobody types it. Typing "coeus" with nothing
// after it opens the terminal screen. "coeus serve" is the agent itself, and
// firstturn.go is the stand-in that answers one message with one model call
// until internal/loop lands.
package main
