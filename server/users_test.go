package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// newUsersServer returns a Server on the test database with its own bootstrap
// keys, plus a cleanup of the accounts created under prefix.
func newUsersServer(t *testing.T) (*Server, string) {
	t.Helper()
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v)", err)
	}
	stamp := time.Now().UnixNano()
	prefix := fmt.Sprintf("u%d", stamp%1_000_000_000)
	s := &Server{
		m:              m,
		jwtKey:         []byte("0123456789abcdef0123456789abcdef"),
		loginLimit:     newLoginLimiter(time.Now),
		pwdLimit:       newLoginLimiter(time.Now),
		authHashKey:    fmt.Sprintf("test.password_hash.%d", stamp),
		authVersionKey: fmt.Sprintf("test.password_version.%d", stamp),
	}
	t.Cleanup(func() {
		_, _ = m.pg.Exec(`DELETE FROM users WHERE username LIKE $1`, prefix+"%")
		_, _ = m.pg.Exec(`DELETE FROM audit_log WHERE username LIKE $1`, prefix+"%")
		_, _ = m.pg.Exec(`DELETE FROM settings WHERE key=$1 OR key=$2`, s.authHashKey, s.authVersionKey)
		m.Close()
	})
	return s, prefix
}

func call(t *testing.T, h http.HandlerFunc, method, path, token, body string, pathID string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if pathID != "" {
		r.SetPathValue("id", pathID)
	}
	w := httptest.NewRecorder()
	h(w, r)
	return w
}

func tokenOf(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.Token == "" {
		t.Fatalf("no token: status=%d body=%s", w.Code, w.Body.String())
	}
	return out.Token
}

func login(t *testing.T, s *Server, user, pass string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": user, "password": pass})
	return call(t, s.authLogin, "POST", "/api/auth/login", "", string(body), "")
}

func TestAdditionalAccountsLifecycle(t *testing.T) {
	s, prefix := newUsersServer(t)

	// Bootstrap account via setup, as on a fresh instance.
	s.setupToken = "setup-ok"
	owner := tokenOf(t, call(t, s.authInit, "POST", "/api/auth/init", "", `{"password":"owner-pass-123","setup_token":"setup-ok"}`, ""))

	alice := prefix + "alice"
	w := call(t, s.usersCreate, "POST", "/api/users", owner, fmt.Sprintf(`{"username":%q,"password":"alice-pass-123"}`, alice), "")
	if w.Code != 201 {
		t.Fatalf("create status=%d body=%s", w.Code, w.Body.String())
	}
	var created struct {
		ID        int64  `json:"id"`
		CreatedBy string `json:"created_by"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created.ID <= 0 || created.CreatedBy != ownerUsername {
		t.Fatalf("created = %+v, want an id and created_by %s", created, ownerUsername)
	}
	id := strconv.FormatInt(created.ID, 10)

	// Case-insensitive duplicate and reserved name.
	if w := call(t, s.usersCreate, "POST", "/api/users", owner, fmt.Sprintf(`{"username":%q,"password":"alice-pass-123"}`, strings.ToUpper(alice)), ""); w.Code != 409 {
		t.Fatalf("duplicate status=%d, want 409", w.Code)
	}
	if w := call(t, s.usersCreate, "POST", "/api/users", owner, `{"username":"artex","password":"whatever-pass-1"}`, ""); w.Code != 400 {
		t.Fatalf("reserved name status=%d, want 400", w.Code)
	}

	// Login, session, attribution.
	if w := login(t, s, alice, "wrong-password-1"); w.Code != 401 {
		t.Fatalf("wrong password status=%d", w.Code)
	}
	if w := login(t, s, prefix+"nobody", "alice-pass-123"); w.Code != 401 {
		t.Fatalf("unknown user status=%d", w.Code)
	}
	aliceTok := tokenOf(t, login(t, s, alice, "alice-pass-123"))
	if !s.sessionOK(aliceTok) {
		t.Fatal("fresh token rejected")
	}
	r := requestWithToken("GET", "/api/x", aliceTok)
	if got := s.approver(r); got != alice {
		t.Fatalf("approver = %q, want %q", got, alice)
	}
	if w := call(t, s.authMe, "GET", "/api/auth/me", aliceTok, "", ""); !strings.Contains(w.Body.String(), `"owner":false`) {
		t.Fatalf("me = %s", w.Body.String())
	}

	// Nobody can disable or delete their own account.
	if w := call(t, s.usersUpdate, "PATCH", "/api/users/"+id, aliceTok, `{"disabled":true}`, id); w.Code != 400 {
		t.Fatalf("self-disable status=%d, want 400", w.Code)
	}
	if w := call(t, s.usersDelete, "DELETE", "/api/users/"+id, aliceTok, "", id); w.Code != 400 {
		t.Fatalf("self-delete status=%d, want 400", w.Code)
	}
	// The bootstrap account is not managed here.
	if w := call(t, s.usersDelete, "DELETE", "/api/users/0", aliceTok, "", "0"); w.Code != 400 {
		t.Fatalf("owner delete status=%d, want 400", w.Code)
	}

	// A reset by another admin revokes only that account's sessions.
	if w := call(t, s.usersUpdate, "PATCH", "/api/users/"+id, owner, `{"password":"alice-pass-456"}`, id); w.Code != 200 {
		t.Fatalf("reset status=%d body=%s", w.Code, w.Body.String())
	}
	if s.sessionOK(aliceTok) {
		t.Fatal("token from before the reset is still accepted")
	}
	if !s.sessionOK(owner) {
		t.Fatal("resetting another account revoked the owner's session")
	}
	aliceTok = tokenOf(t, login(t, s, alice, "alice-pass-456"))

	// Self password change revokes the old token; the new password logs in.
	w = call(t, s.authChangePassword, "POST", "/api/auth/change-password", aliceTok, `{"old_password":"alice-pass-456","new_password":"alice-pass-789"}`, "")
	if w.Code != 200 {
		t.Fatalf("self change status=%d body=%s", w.Code, w.Body.String())
	}
	if s.sessionOK(aliceTok) {
		t.Fatal("token from before the self change is still accepted")
	}
	aliceTok = tokenOf(t, login(t, s, alice, "alice-pass-789"))

	// Disable revokes and blocks login; enable lets it back in.
	if w := call(t, s.usersUpdate, "PATCH", "/api/users/"+id, owner, `{"disabled":true}`, id); w.Code != 200 {
		t.Fatalf("disable status=%d", w.Code)
	}
	if s.sessionOK(aliceTok) {
		t.Fatal("disabled account still has a session")
	}
	if w := login(t, s, alice, "alice-pass-789"); w.Code != 403 {
		t.Fatalf("disabled login status=%d, want 403", w.Code)
	}
	if w := call(t, s.usersUpdate, "PATCH", "/api/users/"+id, owner, `{"disabled":false}`, id); w.Code != 200 {
		t.Fatalf("enable status=%d", w.Code)
	}
	aliceTok = tokenOf(t, login(t, s, alice, "alice-pass-789"))

	// A mutation through requireAuth lands in the audit log under the account.
	s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })).
		ServeHTTP(httptest.NewRecorder(), requestWithToken("POST", "/api/tasks", aliceTok))
	deadline := time.Now().Add(3 * time.Second)
	for {
		entries, err := s.m.pg.ListAudit(50, alice)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, e := range entries {
			if e.Action == "POST" && e.Path == "/api/tasks" && e.Status == 204 {
				found = true
			}
		}
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("audit row for %s not found: %+v", alice, entries)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Delete revokes.
	if w := call(t, s.usersDelete, "DELETE", "/api/users/"+id, owner, "", id); w.Code != 200 {
		t.Fatalf("delete status=%d", w.Code)
	}
	if s.sessionOK(aliceTok) {
		t.Fatal("deleted account still has a session")
	}
	if w := login(t, s, alice, "alice-pass-789"); w.Code != 401 {
		t.Fatalf("deleted login status=%d, want 401", w.Code)
	}
}

func TestUserTokenNeedsCacheEntry(t *testing.T) {
	s := &Server{jwtKey: []byte("0123456789abcdef0123456789abcdef")}
	tok, err := signUserJWT(s.jwtKey, 42, "bob", 3)
	if err != nil {
		t.Fatal(err)
	}
	if s.sessionOK(tok) {
		t.Fatal("a user token was accepted without a loaded account")
	}
}

func TestValidateUsername(t *testing.T) {
	for _, ok := range []string{"ana", "jose.hurtado", "op-01", "A_b"} {
		if msg := validateUsername(ok); msg != "" {
			t.Errorf("%q rejected: %s", ok, msg)
		}
	}
	for _, bad := range []string{"", "ab", "ARTEX", "artex", "-lead", "with space", strings.Repeat("x", 33), "ñandú"} {
		if validateUsername(bad) == "" {
			t.Errorf("%q accepted", bad)
		}
	}
}
