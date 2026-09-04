// Package main is the nerdgenie binary: one program that holds the queue, the loop,
// the permissions, the records, the memory, the jobs, and one database file.
//
// This file, the subcommand table in main.go, and serve.go belong to the
// orchestrator, so that no two workers ever edit the same file. A worker who
// adds a subcommand writes a new file in this folder holding one subcommand
// value, and the orchestrator adds that value to the table. A worker who adds a
// slash command exports it as a contract.Command value from its own package, and
// the orchestrator registers it in serve.go.
//
// The table is version, help, init, doctor, serve, run, tui, install, uninstall,
// signal, backup, restore, replay, update, askpass, and the sandbox helper,
// which is hidden from the listing because the fence starts it and nobody types
// it. Typing "nerdgenie" with nothing after it opens the terminal screen.
//
// "nerdgenie serve" is the agent itself. Its wiring is serve.go for the stores and
// the serving loops, wiring.go for everything a message meets on its way in and
// out, model.go for the model chain, status.go for what a screen is told,
// previews.go for the questions waiting to be answered, skillsbox.go and
// signalchannel.go for two knots the order of building ties, watchedmodel.go for
// what each model call cost, timedstore.go so that every event carries a time,
// and runlock.go so that only one copy runs on one home folder.
package main
