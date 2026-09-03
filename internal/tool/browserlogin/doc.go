// Package browserlogin is the browser_login tool: it fills a login form from
// the vault without the model ever seeing a credential.
//
// The model does two things and no more: it names the site, and it points at the
// boxes on the page. Everything else happens on the other side of a function the
// harness supplies, which wave five wires to the vault. The username, the
// password, and the two-factor code go straight from that function into the
// browser worker, which types them; nothing carrying one of them is returned,
// logged, or put anywhere the model can read. That is the whole point of the
// tool, and the tests hold it to that by refusing any result that carries a
// credential in it.
package browserlogin
