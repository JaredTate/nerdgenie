// The way the codex program's sign-in file is read, and the way a token's
// expiry and account are read out of its claims, were read from Hermes at
// ~/.hermes/hermes-agent/hermes_cli/auth.py (_import_codex_cli_tokens and
// _codex_access_token_is_expiring) and
// ~/.hermes/hermes-agent/agent/auxiliary_client.py, where the account
// identifier is dug out of the token for the ChatGPT-Account-Id header. That
// is the installed Hermes; the reference clone at ~/Code/hermes-agent is older
// than its Codex sign-in and has no copy of it. Hermes then keeps a copy of
// the tokens and refreshes them itself; this reads the file and nothing more,
// so that the codex program stays the one owner of the sign-in.

package provider

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// The bound on reading the sign-in file, and the one thing every sign-in
// failure tells the user to do.
const (
	// maxCodexLoginBytes caps how much of the sign-in file is read. The real
	// file is a few kilobytes, and a file a hundred times that size is not
	// the codex program's.
	maxCodexLoginBytes = 1 << 20
	// codexSignInHint is what every sign-in failure ends with, because the
	// codex program is the one thing that can put the file right.
	codexSignInHint = "run codex once to sign in and try again"
)

// codexLogin is what the provider needs from the sign-in file: the token to
// send, the account it belongs to, and when it stops working. The token is
// never written to a log or an error.
type codexLogin struct {
	// AccessToken is the bearer token the backend wants on every call.
	AccessToken string
	// AccountID is the ChatGPT account the token belongs to, which the backend
	// wants in a header of its own.
	AccountID string
	// Expires is when the token stops working, or the zero time when the
	// token does not say.
	Expires time.Time
}

// codexLoginFile is the shape of the file the codex program writes, as far as
// this provider reads it.
type codexLoginFile struct {
	// Tokens is the object the program keeps its sign-in under.
	Tokens struct {
		AccessToken string `json:"access_token"`
		AccountID   string `json:"account_id"`
	} `json:"tokens"`
}

// codexTokenClaims are the two claims this provider reads out of the token:
// when it expires, and which account it belongs to.
type codexTokenClaims struct {
	// Expiry is the moment the token stops working, as seconds since 1970.
	Expiry float64 `json:"exp"`
	// Auth holds the account identifier under the key OpenAI writes it with.
	Auth struct {
		ChatGPTAccountID string `json:"chatgpt_account_id"`
	} `json:"https://api.openai.com/auth"`
}

// readCodexLogin reads the sign-in the codex program keeps at the path and
// refuses one that is missing, unreadable, not the program's shape, or expired
// at the given moment. Every message says what to do, and none carries the
// token.
func readCodexLogin(path string, now time.Time) (codexLogin, error) {
	contents, err := readFileUpTo(path, maxCodexLoginBytes)
	if errors.Is(err, os.ErrNotExist) {
		return codexLogin{}, fmt.Errorf("the codex sign-in file %s does not exist, so %s", path, codexSignInHint)
	}
	if err != nil {
		return codexLogin{}, fmt.Errorf("the codex sign-in file %s could not be read, so check that it is yours to read: %w", path, err)
	}
	file := codexLoginFile{}
	if err := json.Unmarshal(contents, &file); err != nil {
		return codexLogin{}, fmt.Errorf("the codex sign-in file %s is not the JSON the codex program writes, so %s", path, codexSignInHint)
	}
	token := strings.TrimSpace(file.Tokens.AccessToken)
	if token == "" {
		return codexLogin{}, fmt.Errorf("the codex sign-in file %s holds no access token, so %s", path, codexSignInHint)
	}
	login := codexLogin{AccessToken: token, AccountID: strings.TrimSpace(file.Tokens.AccountID)}
	claims := claimsOfToken(token)
	if claims.Expiry > 0 {
		login.Expires = time.Unix(int64(claims.Expiry), 0).UTC()
		if !now.Before(login.Expires) {
			return codexLogin{}, fmt.Errorf("the codex sign-in in %s expired at %s, so %s",
				path, login.Expires.Format(time.RFC3339), codexSignInHint)
		}
	}
	if login.AccountID == "" {
		login.AccountID = strings.TrimSpace(claims.Auth.ChatGPTAccountID)
	}
	return login, nil
}

// readFileUpTo reads a file whole, but never more than the limit.
func readFileUpTo(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, limit))
}

// claimsOfToken reads the claims out of the middle part of a token, and comes
// back empty for a token that is not shaped that way, because the backend is
// the judge of a token this provider cannot read and answers with a refusal
// that names the problem.
func claimsOfToken(token string) codexTokenClaims {
	claims := codexTokenClaims{}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return claims
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return claims
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return codexTokenClaims{}
	}
	return claims
}
