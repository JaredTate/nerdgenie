// Package browserread is the browser_read tool, and it owns what a web page
// looks like to the model.
//
// The model never sees a picture of a page or the markup behind one. It sees a
// short outline: the address and the title, then one line per element with the
// short reference it points at later, the kind of thing it is, and what it is
// called; a count of how much is below the fold; a plain line for a dialog
// box, a download, or one of the three walls that hand the browser to the user;
// and last what the page says, its visible text quoted line by line with a
// table as one line per row, because a number in a cell of a table is on no
// element at all. A few hundred tokens for the outline and a bounded few
// thousand for the text, cut before anything else when the result would pass
// the cap, which is what makes acting on a page cost about the same on the
// fortieth step as on the first. Every other browser tool describes the page it
// left behind in these same words, by calling the two functions here, so that a
// page reads the same however the model arrived at it.
package browserread
