#!/usr/bin/env bash
# Check that every Go file this repository tracks is gofmt clean.
#
# The list comes from `git ls-files` rather than from `find`, because find
# descends into the worktrees other workers keep under .claude/, and those files
# belong to their branches rather than to this one. Every path is quoted, because
# a folder with a space in its name would otherwise reach gofmt as two arguments
# and make it fail on files that are not there. gofmt's exit status is checked as
# well as its output, because a file it cannot parse makes it exit 2 while
# listing nothing at all.
set -uo pipefail

cd "$(dirname "$0")/.."

# Git exports GIT_DIR and GIT_WORK_TREE to the programs it runs, and GIT_DIR
# outranks the working directory, so a run inside a hook would otherwise list
# some other repository's files.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE

tracked=()
while IFS= read -r -d '' path; do
	tracked+=("$path")
done < <(git ls-files -z -- '*.go')

if [ "${#tracked[@]}" -eq 0 ]; then
	echo "this repository tracks no Go files, so there is nothing to check"
	exit 0
fi

unformatted="$(gofmt -l -- "${tracked[@]}" 2>&1)"
status=$?

if [ "$status" -ne 0 ]; then
	echo "gofmt could not read every file; it exited $status and said:"
	echo "$unformatted"
	exit 1
fi
if [ -n "$unformatted" ]; then
	echo "these files are not gofmt clean; run gofmt -w on them:"
	echo "$unformatted"
	exit 1
fi
