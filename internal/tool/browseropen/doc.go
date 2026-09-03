// Package browseropen is the browser_open tool: it takes the agent's own Chrome
// to a web address and hands back the page as the model reads it.
//
// It is the way into every other browser tool, because the references the model
// clicks and types into come from a page it has read. The address is checked for
// being an ordinary web address before the browser is asked to go anywhere, and
// the page that comes back is described in the words the browser_read tool owns,
// so that a page reads the same however the model arrived at it.
package browseropen
