// Package browserread is the browser_read tool, and it owns what a web page
// looks like to the model.
//
// The model never sees a picture of a page or the markup behind one. It sees a
// short outline: the address and the title, then one line per element with the
// short reference it points at later, the kind of thing it is, and what it is
// called; a count of how much is below the fold; and a plain line for a dialog
// box, a download, or one of the three walls that hand the browser to the user.
// A few hundred tokens in all, which is what makes acting on a page cost about
// the same on the fortieth step as on the first. Every other browser tool
// describes the page it left behind in these same words, by calling the two
// functions here, so that a page reads the same however the model arrived at it.
package browserread
