// Package main is runreport, the development script that prints the numbers
// of one task's run out of the event log: the rounds and the minutes, the
// calls by tool, how many replies asked for more than one tool, how many
// rounds only wrote the record and how many only ran the tests, the tokens
// in and cached and out, and the rewinds and failures the record holds.
//
// They are the numbers the fifth Tetris build was measured by hand with on
// 5 September 2026, when they showed that forty percent of the rounds did no
// work on the world, and they are what every change to the harness is judged
// against from then on: a change with no number before and after is a guess.
// Run it as `go run ./scripts/runreport --log <nerdgenie.db> [--task N]`,
// on a copy of a live log rather than the file the agent is writing.
package main
