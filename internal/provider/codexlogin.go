// The way the codex program's saved login is read, and the two-minute skew that
// treats a token about to expire as already expired, were borrowed from Hermes
// Agent's _codex_access_token_is_expiring and _decode_jwt_claims in
// ~/Code/hermes-agent/hermes_cli/auth.py. Hermes reads the same file to import
// the login into its own store; Coeus only reads it, every time, and never
// writes it, because the codex program owns that file and refreshes it itself.

package provider

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"
)

// The bounds the login read works inside.
const (
	// codexLoginFileLimit is the most the login file may hold. The real file
	// is a few kilobytes, so anything past a megabyte is not that file.
	codexLoginFileLimit = 1 << 20

	// codexLoginExpirySkew is how far ahead of its expiry a token is already
	// treated as expired, so a request never goes out on a token that runs out
	// in the middle of it. Hermes uses the same two minutes.
	codexLoginExpirySkew = 2 * time.Minute
)

// codexSignInAdvice is what every refusal tells the user to do, because the
// codex program is the only thing that can sign in or refresh the login.
const codexSignInAdvice = "run codex once to sign in or refresh"

// codexLogin is what the codex program saved when the user signed in.
type codexLogin struct {
	// AccessToken is the bearer token the codex program presents to the backend.
	AccessToken string
	// AccountID is the ChatGPT account the token was issued for.
	AccountID string
	// Expires is when the token stops working, read from the token itself.
	Expires time.Time
}

// codexLoginFile is the part of the codex program's login file that Coeus
// reads. The file also holds the sign-in mode, an id token, a refresh token,
// and the time of the last refresh, none of which Coeus needs.
type codexLoginFile struct {
	// Tokens holds the two values Coeus needs, under the keys the codex
	// program writes them with.
	Tokens struct {
		AccessToken string `json:"access_token"`
		AccountID   string `json:"account_id"`
	} `json:"tokens"`
}

// readCodexLogin reads the codex program's login file at path. It refuses a
// missing, unreadable, oversized, or malformed file, a file without an access
// token or an account id, and a token that is expired or within the skew of
// expiring. No error it returns carries the token, the account id, or any
// other part of the file.
func readCodexLogin(path string, now time.Time) (codexLogin, error) {
	contents, err := readCodexLoginFile(path)
	if err != nil {
		return codexLogin{}, err
	}

	var file codexLoginFile
	if err := json.Unmarshal(contents, &file); err != nil {
		// The decoder's own message can quote a character from the file, so
		// it is left out rather than wrapped.
		return codexLogin{}, fmt.Errorf("the codex login file %s is not the JSON the codex program writes; %s", path, codexSignInAdvice)
	}
	if file.Tokens.AccessToken == "" {
		return codexLogin{}, fmt.Errorf("the codex login file %s holds no access token; %s", path, codexSignInAdvice)
	}
	if file.Tokens.AccountID == "" {
		return codexLogin{}, fmt.Errorf("the codex login file %s holds no account id; %s", path, codexSignInAdvice)
	}

	expires, err := codexTokenExpiry(file.Tokens.AccessToken)
	if err != nil {
		return codexLogin{}, fmt.Errorf("the access token in the codex login file %s %s; %s", path, err, codexSignInAdvice)
	}
	if !expires.After(now.Add(codexLoginExpirySkew)) {
		return codexLogin{}, fmt.Errorf("the access token in the codex login file %s expired at %s or will within %s; %s",
			path, expires.UTC().Format(time.RFC3339), codexLoginExpirySkew, codexSignInAdvice)
	}

	return codexLogin{
		AccessToken: file.Tokens.AccessToken,
		AccountID:   file.Tokens.AccountID,
		Expires:     expires,
	}, nil
}

// readCodexLoginFile reads the whole login file, refusing one larger than the
// limit rather than reading past it.
func readCodexLoginFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("no codex login file at %s, so the codex program has not signed in on this machine; %s", path, codexSignInAdvice)
	}
	if err != nil {
		return nil, fmt.Errorf("the codex login file could not be opened: %w; %s", err, codexSignInAdvice)
	}
	defer file.Close()

	contents, err := io.ReadAll(io.LimitReader(file, codexLoginFileLimit+1))
	if err != nil {
		return nil, fmt.Errorf("the codex login file %s could not be read: %w; %s", path, err, codexSignInAdvice)
	}
	if len(contents) > codexLoginFileLimit {
		return nil, fmt.Errorf("the codex login file %s is larger than %d bytes, which the codex program never writes; %s", path, codexLoginFileLimit, codexSignInAdvice)
	}
	return contents, nil
}

// codexTokenExpiry reads the expiry out of the access token, which is a JSON
// web token: three base64url parts joined by dots, the middle one a JSON object
// whose exp claim is the expiry in Unix seconds. The signature is not checked,
// because Coeus is not the party the token is meant to convince; it only needs
// to know when the token runs out. The error it returns finishes the sentence
// "the access token ..." and never quotes the token.
func codexTokenExpiry(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("is not the three-part token the codex program saves")
	}

	// The standard leaves the padding off, but a writer that keeps it is
	// still readable once the padding is stripped.
	claimsText, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return time.Time{}, fmt.Errorf("has a middle part that is not base64url")
	}

	var claims struct {
		Expiry *float64 `json:"exp"`
	}
	if err := json.Unmarshal(claimsText, &claims); err != nil {
		return time.Time{}, fmt.Errorf("has claims that do not decode as a JSON object with a numeric exp")
	}
	if claims.Expiry == nil {
		return time.Time{}, fmt.Errorf("has no exp claim to read an expiry from")
	}
	return time.Unix(int64(*claims.Expiry), 0), nil
}
