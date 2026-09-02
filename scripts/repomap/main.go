package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run generates the map for the repository named by "--root", or for the current
// folder when no root is given, and prints it. It returns zero on success, one
// when the map could not be generated, and two when the command line was wrong.
func run(arguments []string, output io.Writer, problems io.Writer) int {
	root := "."
	for at := 0; at < len(arguments); at++ {
		if arguments[at] == "--root" && at+1 < len(arguments) {
			root = arguments[at+1]
			at++
			continue
		}
		fmt.Fprintln(problems, "usage: repomap [--root <folder>]; it prints REPO_MAP.md on standard output")
		return 2
	}

	generated, err := generate(root)
	if err != nil {
		fmt.Fprintln(problems, err)
		return 1
	}
	if _, err := io.WriteString(output, generated); err != nil {
		fmt.Fprintln(problems, err)
		return 1
	}
	return 0
}
