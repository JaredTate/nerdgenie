package update

import (
	"strconv"
	"strings"
)

// maxVersionParts is how many dotted numbers a version is read as. Three is
// major, minor, and patch, and nothing published has ever needed a fourth.
const maxVersionParts = 3

// DevelopmentVersion is what a build made outside a release calls itself. It is
// older than every published version, so that a developer running "nerdgenie update"
// on their own build installs the release rather than being told there is
// nothing to do.
const DevelopmentVersion = "dev"

// Newer says whether the candidate version is above the running one.
//
// A version is read as up to three dotted numbers with an optional leading v,
// and anything after a hyphen is a pre-release, which sits below the same
// numbers without one. A version this cannot read at all, such as the word dev
// that an unreleased build carries, counts as older than every version it can.
func Newer(running string, candidate string) bool {
	runningParts, runningPreRelease, runningRead := readVersion(running)
	candidateParts, candidatePreRelease, candidateRead := readVersion(candidate)
	if !candidateRead {
		return false
	}
	if !runningRead {
		return true
	}
	for at := range maxVersionParts {
		if candidateParts[at] != runningParts[at] {
			return candidateParts[at] > runningParts[at]
		}
	}
	return runningPreRelease != "" && candidatePreRelease == ""
}

// readVersion pulls the numbers and the pre-release out of a version, and says
// whether it could read it at all.
func readVersion(version string) ([maxVersionParts]int, string, bool) {
	numbers := [maxVersionParts]int{}
	written := strings.TrimSpace(version)
	written = strings.TrimPrefix(written, "v")
	preRelease := ""
	if at := strings.IndexAny(written, "-+"); at >= 0 {
		preRelease = written[at+1:]
		written = written[:at]
	}
	if written == "" {
		return numbers, "", false
	}

	parts := strings.Split(written, ".")
	if len(parts) > maxVersionParts {
		return numbers, "", false
	}
	for at, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return numbers, "", false
		}
		numbers[at] = number
	}
	return numbers, preRelease, true
}
