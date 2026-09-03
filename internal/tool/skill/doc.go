// Package skill is the skill tool: it shows a saved procedure, runs one, or
// writes a new one down.
//
// A skill is a folder holding a description, the steps or a script, a dry-run
// test, and a changelog, and everything about how that folder is kept and
// replayed belongs to the skill package behind the contract. This tool only
// reads the model's three words and calls the one method each of them means.
// Only the names and one-line descriptions of skills ride in the prompt, so
// viewing one is how the model reads the body when it needs it.
package skill
