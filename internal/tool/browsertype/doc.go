// Package browsertype is the browser_type tool: it types into one element and
// checks that what the model expected actually happened.
//
// Typing nothing at all is allowed, because clearing a field is a thing a person
// does. Typing a password is not done here: the browser_login tool fills a login
// form from the vault, and the model never sees or writes a credential. What
// comes back is what changed on the page and the page as it stands, in the words
// the browser_read tool owns.
package browsertype
