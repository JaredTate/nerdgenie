package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/JaredTate/nerdgenie/internal/log"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// theUsage is the line said for a wrong or missing argument.
const theUsage = "usage: runreport --log <nerdgenie.db> [--task <number> | --from <number>]; it prints one task's numbers, or the sum from one task to the newest"

// run prints the numbers of one task's run out of the log named by "--log", for
// the task named by "--task", the newest one, or every task from the one named
// by "--from" to the newest together. It returns zero on success, one when the
// log could not be read, and two when the command line was wrong.
func run(arguments []string, output io.Writer, problems io.Writer) int {
	path, taskID, from := "", "", ""
	for at := 0; at < len(arguments); at++ {
		switch {
		case arguments[at] == "--log" && at+1 < len(arguments):
			path = arguments[at+1]
			at++
		case arguments[at] == "--task" && at+1 < len(arguments):
			taskID = arguments[at+1]
			at++
		case arguments[at] == "--from" && at+1 < len(arguments):
			from = arguments[at+1]
			at++
		default:
			fmt.Fprintln(problems, theUsage)
			return 2
		}
	}
	if path == "" {
		fmt.Fprintln(problems, theUsage)
		return 2
	}
	if from != "" {
		number := 0
		if _, err := fmt.Sscanf(from, "%d", &number); err != nil || number < 1 {
			fmt.Fprintln(problems, theUsage)
			return 2
		}
	}
	ctx := context.Background()
	store, err := log.Open(ctx, path)
	if err != nil {
		fmt.Fprintln(problems, err)
		return 1
	}
	defer store.Close()
	numbers, err := measure(ctx, store, taskID)
	if from != "" {
		numbers, err = measureFrom(ctx, store, from)
	}
	if err != nil {
		fmt.Fprintln(problems, err)
		return 1
	}
	fmt.Fprintln(output, numbers)
	return 0
}
