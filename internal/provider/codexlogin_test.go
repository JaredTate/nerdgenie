package provider

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The pieces of a made-up login. Each one is a distinctive string so a test
// can tell when an error message has leaked it.
const (
	testCodexAccountID    = "acct-secret-9f8e7d6c"
	testCodexRefreshToken = "refresh-secret-5a4b3c2d"
	testCodexIDToken      = "id-secret-1e2f3a4b"
)

// codexTestNow is the fixed clock every test reads the file against.
var codexTestNow = time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC)

// codexTestToken builds a signed-looking token whose middle part carries the
// claims given, the way the codex program's access token does.
func codexTestToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	body, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("encoding the test claims failed: %v", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(body)
	signature := base64.RawURLEncoding.EncodeToString([]byte("signature-secret-7c8d9e0f"))
	return header + "." + payload + "." + signature
}

// codexTestFile writes the shape the codex program keeps, with the token and
// account id given, and returns its path. An empty token or account id leaves
// that key out of the file.
func codexTestFile(t *testing.T, accessToken string, accountID string) string {
	t.Helper()
	tokens := map[string]any{
		"id_token":      testCodexIDToken,
		"refresh_token": testCodexRefreshToken,
	}
	if accessToken != "" {
		tokens["access_token"] = accessToken
	}
	if accountID != "" {
		tokens["account_id"] = accountID
	}
	contents, err := json.Marshal(map[string]any{
		"auth_mode":      "chatgpt",
		"OPENAI_API_KEY": nil,
		"tokens":         tokens,
		"last_refresh":   "2026-09-02T21:41:13.593042070Z",
	})
	if err != nil {
		t.Fatalf("encoding the test login file failed: %v", err)
	}
	return codexTestFileHolding(t, string(contents))
}

// codexTestFileHolding writes exactly the text given as a login file.
func codexTestFileHolding(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing the test login file failed: %v", err)
	}
	return path
}

// mustNotLeak fails when the error names any piece of the login: the token,
// the account id, or the other secrets the file holds.
func mustNotLeak(t *testing.T, err error, token string) {
	t.Helper()
	if err == nil {
		return
	}
	message := err.Error()
	secrets := append(strings.Split(token, "."), token, testCodexAccountID, testCodexRefreshToken, testCodexIDToken)
	for _, secret := range secrets {
		if secret != "" && strings.Contains(message, secret) {
			t.Errorf("the error message leaks a secret from the login file: %q", message)
		}
	}
}

// mustAdviseSigningIn fails when the error does not tell the user what to do.
func mustAdviseSigningIn(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error and got none")
	}
	if !strings.Contains(err.Error(), "run codex once") {
		t.Errorf("the error does not tell the user to run codex once to sign in or refresh: %v", err)
	}
}

func TestReadCodexLoginReturnsTheSavedTokenAndAccount(t *testing.T) {
	expires := codexTestNow.Add(time.Hour)
	token := codexTestToken(t, map[string]any{"exp": expires.Unix(), "sub": "somebody"})
	path := codexTestFile(t, token, testCodexAccountID)

	login, err := readCodexLogin(path, codexTestNow)
	if err != nil {
		t.Fatalf("reading a fresh login failed: %v", err)
	}
	if login.AccessToken != token {
		t.Errorf("access token: got %q, want the token in the file", login.AccessToken)
	}
	if login.AccountID != testCodexAccountID {
		t.Errorf("account id: got %q, want %q", login.AccountID, testCodexAccountID)
	}
	if !login.Expires.Equal(expires) {
		t.Errorf("expiry: got %v, want %v", login.Expires, expires)
	}
}

func TestReadCodexLoginLeavesTheFileAlone(t *testing.T) {
	token := codexTestToken(t, map[string]any{"exp": codexTestNow.Add(time.Hour).Unix()})
	path := codexTestFile(t, token, testCodexAccountID)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the test file back failed: %v", err)
	}
	if _, err := readCodexLogin(path, codexTestNow); err != nil {
		t.Fatalf("reading a fresh login failed: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the test file back failed: %v", err)
	}
	if string(before) != string(after) {
		t.Error("reading the login changed the file, and it must only be read")
	}
}

func TestReadCodexLoginRefusesAnExpiredOrExpiringToken(t *testing.T) {
	cases := []struct {
		name    string
		exp     time.Time
		wantErr bool
	}{
		{"an hour ago", codexTestNow.Add(-time.Hour), true},
		{"a second ago", codexTestNow.Add(-time.Second), true},
		{"right now", codexTestNow, true},
		{"inside the skew", codexTestNow.Add(codexLoginExpirySkew - time.Second), true},
		{"at the edge of the skew", codexTestNow.Add(codexLoginExpirySkew), true},
		{"just past the skew", codexTestNow.Add(codexLoginExpirySkew + time.Second), false},
		{"an hour from now", codexTestNow.Add(time.Hour), false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			token := codexTestToken(t, map[string]any{"exp": testCase.exp.Unix()})
			path := codexTestFile(t, token, testCodexAccountID)
			login, err := readCodexLogin(path, codexTestNow)
			mustNotLeak(t, err, token)
			if !testCase.wantErr {
				if err != nil {
					t.Fatalf("a token good for %s was refused: %v", testCase.exp.Sub(codexTestNow), err)
				}
				return
			}
			mustAdviseSigningIn(t, err)
			if !strings.Contains(err.Error(), "expired") {
				t.Errorf("the error does not say the token has expired: %v", err)
			}
			if login != (codexLogin{}) {
				t.Errorf("a refused login still carried values: %+v", login)
			}
		})
	}
}

func TestReadCodexLoginRefusesAFileItCannotRead(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "auth.json")
		_, err := readCodexLogin(path, codexTestNow)
		mustAdviseSigningIn(t, err)
		if !strings.Contains(err.Error(), "no codex login file") {
			t.Errorf("the error does not say the file is missing: %v", err)
		}
	})
	t.Run("a folder rather than a file", func(t *testing.T) {
		_, err := readCodexLogin(t.TempDir(), codexTestNow)
		mustAdviseSigningIn(t, err)
	})
	t.Run("not JSON", func(t *testing.T) {
		path := codexTestFileHolding(t, "not json at all "+testCodexAccountID)
		_, err := readCodexLogin(path, codexTestNow)
		mustAdviseSigningIn(t, err)
		mustNotLeak(t, err, "")
	})
	t.Run("over one megabyte", func(t *testing.T) {
		token := codexTestToken(t, map[string]any{"exp": codexTestNow.Add(time.Hour).Unix()})
		padding := strings.Repeat(" ", codexLoginFileLimit)
		path := codexTestFileHolding(t, fmt.Sprintf(`{"tokens":{"access_token":%q,"account_id":%q}}%s`, token, testCodexAccountID, padding))
		_, err := readCodexLogin(path, codexTestNow)
		mustAdviseSigningIn(t, err)
		mustNotLeak(t, err, token)
		if !strings.Contains(err.Error(), "larger than") {
			t.Errorf("the error does not say the file is too large: %v", err)
		}
	})
}

func TestReadCodexLoginRefusesAFileMissingAKey(t *testing.T) {
	token := codexTestToken(t, map[string]any{"exp": codexTestNow.Add(time.Hour).Unix()})
	t.Run("no access token", func(t *testing.T) {
		path := codexTestFile(t, "", testCodexAccountID)
		_, err := readCodexLogin(path, codexTestNow)
		mustAdviseSigningIn(t, err)
		mustNotLeak(t, err, token)
		if !strings.Contains(err.Error(), "access token") {
			t.Errorf("the error does not name the missing access token: %v", err)
		}
	})
	t.Run("no account id", func(t *testing.T) {
		path := codexTestFile(t, token, "")
		_, err := readCodexLogin(path, codexTestNow)
		mustAdviseSigningIn(t, err)
		mustNotLeak(t, err, token)
		if !strings.Contains(err.Error(), "account id") {
			t.Errorf("the error does not name the missing account id: %v", err)
		}
	})
	t.Run("no tokens at all", func(t *testing.T) {
		path := codexTestFileHolding(t, `{"auth_mode":"chatgpt","OPENAI_API_KEY":null}`)
		_, err := readCodexLogin(path, codexTestNow)
		mustAdviseSigningIn(t, err)
	})
}

func TestReadCodexLoginRefusesATokenThatDoesNotDecode(t *testing.T) {
	encode := func(text string) string { return base64.RawURLEncoding.EncodeToString([]byte(text)) }
	cases := map[string]string{
		"two parts only":            encode(`{"alg":"none"}`) + "." + encode(`{"exp":4102444800}`),
		"four parts":                codexTestToken(t, map[string]any{"exp": 4102444800}) + ".extra",
		"middle part not base64":    encode(`{"alg":"none"}`) + ".!!!not-base64!!!." + encode("signature"),
		"middle part not JSON":      encode(`{"alg":"none"}`) + "." + encode("not json") + "." + encode("signature"),
		"middle part not an object": encode(`{"alg":"none"}`) + "." + encode(`[1,2,3]`) + "." + encode("signature"),
		"no expiry claim":           codexTestToken(t, map[string]any{"sub": "somebody"}),
		"expiry is not a number":    codexTestToken(t, map[string]any{"exp": "tomorrow"}),
		"padded middle part":        encode(`{"alg":"none"}`) + "." + base64.URLEncoding.EncodeToString([]byte(`{"exp":"soon"}`)) + "." + encode("signature"),
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			path := codexTestFile(t, token, testCodexAccountID)
			_, err := readCodexLogin(path, codexTestNow)
			mustAdviseSigningIn(t, err)
			mustNotLeak(t, err, token)
		})
	}
}

func TestReadCodexLoginAcceptsAPaddedToken(t *testing.T) {
	// A token whose middle part carries base64 padding still decodes, because
	// the standard says the padding is left off but not every writer obeys it.
	expires := codexTestNow.Add(time.Hour)
	header := base64.URLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload := base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, expires.Unix())))
	token := header + "." + payload + ".sig"
	path := codexTestFile(t, token, testCodexAccountID)
	login, err := readCodexLogin(path, codexTestNow)
	if err != nil {
		t.Fatalf("a padded token was refused: %v", err)
	}
	if !login.Expires.Equal(expires) {
		t.Errorf("expiry: got %v, want %v", login.Expires, expires)
	}
}

func TestReadCodexLoginNeverPutsTheLoginInAnError(t *testing.T) {
	// Every way the read can fail is tried against a file that holds a real-
	// looking token and account id, and none of the messages may carry them.
	expired := codexTestToken(t, map[string]any{"exp": codexTestNow.Add(-time.Hour).Unix()})
	broken := "broken-secret-header.broken-secret-payload.broken-secret-signature"
	paths := map[string]string{
		"expired":          codexTestFile(t, expired, testCodexAccountID),
		"broken":           codexTestFile(t, broken, testCodexAccountID),
		"no account":       codexTestFile(t, expired, ""),
		"not JSON":         codexTestFileHolding(t, expired+" "+testCodexAccountID),
		"tokens not a map": codexTestFileHolding(t, fmt.Sprintf(`{"tokens":%q}`, expired)),
	}
	for name, path := range paths {
		t.Run(name, func(t *testing.T) {
			_, err := readCodexLogin(path, codexTestNow)
			if err == nil {
				t.Fatal("expected an error and got none")
			}
			mustNotLeak(t, err, expired)
			mustNotLeak(t, err, broken)
		})
	}
}

func TestReadCodexLoginRefusesAFileItMayNotOpen(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can open a file with no permission bits, so this case cannot be made")
	}
	token := codexTestToken(t, map[string]any{"exp": codexTestNow.Add(time.Hour).Unix()})
	path := codexTestFile(t, token, testCodexAccountID)
	if err := os.Chmod(path, 0); err != nil {
		t.Fatalf("taking the permissions off the test file failed: %v", err)
	}
	_, err := readCodexLogin(path, codexTestNow)
	mustAdviseSigningIn(t, err)
	mustNotLeak(t, err, token)
	if !strings.Contains(err.Error(), "could not be opened") {
		t.Errorf("the error does not say the file could not be opened: %v", err)
	}
}
