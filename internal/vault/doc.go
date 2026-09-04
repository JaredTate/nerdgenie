// Package vault is the encrypted secret store: the file, the resolver, the
// two-factor code, the sudo password, the masked prompt, and redaction.
//
// Everything the agent knows that must never reach a language model lives in
// one age-encrypted file, ~/.nerdgenie/vault.age, opened by a private key in
// ~/.nerdgenie/vault.key that only the agent's own user account can read. A secret
// enters the vault in one way, through a prompt in the terminal that shows an
// asterisk for each character typed, and it leaves in three: the harness
// resolves a "secret://name" reference when it fills a login form, the harness
// asks for the sudo password when the user has approved a command that needs
// it, and the redactor blacks out every value it holds in text on its way out
// of the program. The model sees the name and never the value.
package vault
