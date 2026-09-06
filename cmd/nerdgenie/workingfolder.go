package main

// theWorkingFolder is the folder the agent works in, by the same rule the tool
// registry uses: the first sandbox root, and the home when there is none. The
// loop lists it in the orientation a fresh window opens with.
func theWorkingFolder(roots []string, home string) string {
	if len(roots) > 0 {
		return roots[0]
	}
	return home
}
