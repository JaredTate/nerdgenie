// Package search is the search tool: one pattern finds both the files whose
// names match it and the lines that match it, inside the folders the agent may
// work in.
//
// It is one tool and one field because it is one question, "where is that",
// and a model made to choose between a name search and a content search
// gets the choice wrong half the time. The pattern is read as a regular
// expression when it is one, and as a name pattern such as "*.md" when it is
// not, and the result says which reading was used. The search runs through
// ripgrep when it is on the machine, because it is a great deal faster on a
// large tree, and through this package's own walk when it is not; the two are
// held to the same answers by running every test in this package both ways. The
// rows are capped at fifty, because a search that returns a thousand rows has
// told the model nothing it can act on.
package search
