package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/JaredTate/coeus/internal/lint"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run checks every folder named on the command line and returns the exit code:
// zero when nothing was found, one when something was, and two when the command
// line was wrong.
func run(patterns []string, output io.Writer, problems io.Writer) int {
	if len(patterns) == 0 {
		fmt.Fprintln(problems, "usage: stylecheck <folder>... , for example: stylecheck ./...")
		return 2
	}

	found := 0
	for _, pattern := range patterns {
		violations, err := lint.CheckTree(folderOf(pattern))
		if err != nil {
			fmt.Fprintln(problems, err)
			return 1
		}
		for _, violation := range violations {
			fmt.Fprintln(output, violation)
			found++
		}
	}

	if found > 0 {
		fmt.Fprintf(output, "%d style violations; fix them or say in your report why the rule is wrong\n", found)
		return 1
	}
	return 0
}

// folderOf turns a Go package pattern into the folder to walk, so that "./..."
// means here and "internal/..." means that folder and everything under it.
func folderOf(pattern string) string {
	folder := strings.TrimSuffix(pattern, "...")
	folder = strings.TrimSuffix(folder, "/")
	if folder == "" || folder == "." {
		return "."
	}
	return folder
}
