package contract

import (
	"context"
	"strings"
)

// SecretReferencePrefix is how the model refers to a secret. The model writes
// the reference and never sees the value behind it.
const SecretReferencePrefix = "secret://"

// RedactedMarker is what the redactor writes in place of a secret it finds in
// text on its way out of the program.
const RedactedMarker = "[redacted]"

// SecretMarker is what a credential prints as, on every path that could turn it
// into text, so that a log line, an error, or a JSON body can never carry a
// value by accident.
const SecretMarker = "[secret]"

// TerminalChannelName is the name of the terminal channel, the only channel that
// may carry a secret, because it is the only one that can hide what is typed.
const TerminalChannelName = "terminal"

// Credential is one login the vault holds. The harness uses it to fill a login
// form; it is never returned to the model and never written to the log.
type Credential struct {
	// Site is the name the user knows the login by, such as "x-account".
	Site string
	// Domains are the hostnames the credential may be typed into. A login on any
	// other domain is refused.
	Domains []string
	// Username is the login name.
	Username string
	// Password is the password.
	Password string
	// TOTPSecret is the shared secret for the site's two-factor code, or empty.
	TOTPSecret string
}

// String prints the marker and never a value, whatever verb printed it.
func (Credential) String() string { return SecretMarker }

// MarshalJSON writes the marker and never a value, so a credential nested in any
// JSON body comes out as "[secret]".
func (Credential) MarshalJSON() ([]byte, error) { return []byte(`"` + SecretMarker + `"`), nil }

// Secrets is the vault: an encrypted file whose key only the agent's own user
// account can read.
type Secrets interface {
	// Resolve turns a "secret://name" reference into the credential behind it,
	// for the harness alone.
	Resolve(ctx context.Context, reference string) (Credential, error)
	// SudoPassword returns the password for a command approved to run with sudo.
	SudoPassword(ctx context.Context) (string, error)
	// Redact replaces every stored secret it finds in the text. Every channel,
	// the log, and the tool registry send their text through it.
	Redact(text string) string
}

// SecretReferenceName reads the name out of a "secret://name" reference. It
// returns false when the text is not a reference or the name is empty or holds
// a space.
func SecretReferenceName(reference string) (string, bool) {
	name, found := strings.CutPrefix(reference, SecretReferencePrefix)
	if !found || name == "" || strings.ContainsAny(name, " \t\r\n") {
		return "", false
	}
	return name, true
}
