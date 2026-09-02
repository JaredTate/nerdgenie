// Package main is the coeus binary: one program that holds the queue, the loop,
// the permissions, the records, the memory, the jobs, and one database file.
//
// This file and the subcommand table in main.go belong to the orchestrator, so
// that no two workers ever edit the same file. A worker who adds a subcommand
// writes a new file in this folder holding one subcommand value, and the
// orchestrator adds that value to the table. A worker who adds a slash command
// exports it as a contract.Command value from its own package, and the
// orchestrator registers it in serve.go, which arrives in wave 3.
//
// In wave 0 the table holds version and help, and the program does nothing else.
package main
