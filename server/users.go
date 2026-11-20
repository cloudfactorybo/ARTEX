package server

import (
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Autumn-27/artex/db"
)

// Additional login accounts. The bootstrap account (ownerUsername) keeps its
// password in settings and its sessions on the global password version; every
// other account lives in the users table with its own version. All accounts
// have the same permissions; tokens carry the username, so approvals and the
// audit log record who acted.

const ownerUsername = "ARTEX"

var usernameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{2,31}$`)

// dummyHash is compared against when the username does not exist, so a login
// takes about as long either way and does not reveal which names exist.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("artex-no-such-account"), bcrypt.DefaultCost)

// validateUsername returns "" when the name is acceptable, else a reason for the user.
func validateUsername(name string) string {
	if !usernameRE.MatchString(name) {
		return "Username must be 3 to 32 characters: letters, digits, dot, dash or underscore, starting with a letter or digit"
	}
	if strings.EqualFold(name, ownerUsername) {
		return "This username is reserved"
	}
	return ""
}

// userSession is what a session check needs about one account.
type userSession struct {
	username string
	version  int64
	disabled bool
}

// userCache mirrors the users table for session checks, which run on every
// request. Entries only come from database reads; an account missing from it
// (the startup load failed, or it was deleted) fails closed until it logs in again.
type userCache struct {
	mu   sync.RWMutex
	byID map[int64]userSession
}

func (c *userCache) put(u *db.User) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byID == nil {
		c.byID = map[int64]userSession{}
	}
	c.byID[u.ID] = userSession{username: u.Username, version: u.PwdVersion, disabled: u.Disabled}
}

func (c *userCache) remove(id int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.byID, id)
}

func (c *userCache) replace(users []*db.User) {
	next := make(map[int64]userSession, len(users))
	for _, u := range users {
		next[u.ID] = userSession{username: u.Username, version: u.PwdVersion, disabled: u.Disabled}
	}
	c.mu.Lock()
	c.byID = next
	c.mu.Unlock()
}

// valid reports whether a token for (id, username, version) is still a session.
func (c *userCache) valid(id int64, username string, version int64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.byID[id]
	return ok && !e.disabled && e.version == version && e.username == username
}

// loadUsers fills the cache at startup. On failure the cache stays empty and
// every additional account must log in again, which re-reads its row.
func (s *Server) loadUsers() {
	if s.m == nil || s.m.pg == nil {
		return
	}
	var users []*db.User
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		if users, err = s.m.pg.ListUsers(); err == nil {
			break
		}
		log.Printf("[auth] users lookup attempt %d/3: %v", attempt, err)
		time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
	}
	if err != nil {
		log.Printf("[auth] users unreadable; additional accounts must log in again: %v", err)
		return
	}
	s.users.replace(users)
	log.Printf("[auth] %d additional account(s) loaded", len(users))
}

// requestClaims returns the validated claims of the request's session token.
func (s *Server) requestClaims(r *http.Request) (*sessionClaims, bool) {
	tok := extractToken(r)
	if !s.sessionOK(tok) {
		return nil, false
	}
	return parseSessionClaims(tok, s.jwtKey)
}

// loginUser is the login path for every username but the bootstrap one.
func (s *Server) loginUser(w http.ResponseWriter, r *http.Request, ip, username, password string) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	u, err := pg.UserByName(username)
	if errors.Is(err, db.ErrUserNotFound) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		writeErr(w, 401, "Incorrect username or password")
		return
	}
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		writeErr(w, 401, "Incorrect username or password")
		return
	}
	s.loginLimit.reset(ip)
	if u.Disabled {
		writeErr(w, 403, "This account is disabled")
		return
	}
	s.users.put(u)
	if err := pg.TouchUserLogin(u.ID); err != nil {
		log.Printf("[auth] record login of %s: %v", u.Username, err)
	}
	s.audit(u.Username, "LOGIN", "/api/auth/login", 200, ip)
	tok, err := signUserJWT(s.jwtKey, u.ID, u.Username, u.PwdVersion)
	if err != nil {
		writeErr(w, 500, "token generation failed")
		return
	}
	writeJSON(w, 200, map[string]any{"token": tok})
}

// changeUserPassword is change-password for an additional account: the current
// password is required, and the bumped version revokes the account's sessions.
func (s *Server) changeUserPassword(w http.ResponseWriter, r *http.Request, claims *sessionClaims, oldPassword, newPassword string) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	u, err := pg.UserByID(claims.Uid)
	if errors.Is(err, db.ErrUserNotFound) {
		writeErr(w, 401, "Unauthorized")
		return
	}
	if err != nil {
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(oldPassword)); err != nil {
		writeErr(w, 401, "Current password is wrong")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, 500, "Password encryption failed")
		return
	}
	updated, err := pg.SetUserPassword(u.ID, string(hash))
	if err != nil {
		writeErr(w, 500, "Save failed: "+err.Error())
		return
	}
	s.passwordChangeReset(r)
	s.users.put(updated)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/auth/me — the account of the request's session.
func (s *Server) authMe(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.requestClaims(r)
	if !ok {
		writeErr(w, 401, "Unauthorized")
		return
	}
	writeJSON(w, 200, map[string]any{"id": claims.Uid, "username": claims.Subject, "owner": claims.Uid == 0})
}

// userView is one row of GET /api/users. The bootstrap account is listed with
// id 0 and owner true; it is managed through change-password and reset-password.sh.
type userView struct {
	*db.User
	Owner bool `json:"owner"`
}

// GET /api/users
func (s *Server) usersList(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	users, err := pg.ListUsers()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := []userView{{User: &db.User{Username: ownerUsername}, Owner: true}}
	for _, u := range users {
		out = append(out, userView{User: u})
	}
	writeJSON(w, 200, map[string]any{"users": out})
}

// POST /api/users {username, password}
func (s *Server) usersCreate(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(w, r, &req); err != nil {
		writeErr(w, 400, "Invalid request format")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if msg := validateUsername(req.Username); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	if msg := validatePassword(req.Password); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, 500, "Password encryption failed")
		return
	}
	u, err := pg.CreateUser(req.Username, string(hash), s.approver(r))
	if errors.Is(err, db.ErrUserExists) {
		writeErr(w, 409, "Username already exists")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.users.put(u)
	writeJSON(w, 201, userView{User: u})
}

// userIDParam parses {id}; ok is false (and a 400 written) when it is not a
// positive integer. The bootstrap account (id 0) is never managed here.
func userIDParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, 400, "The initial ARTEX account is managed with change password and reset-password.sh")
		return 0, false
	}
	return id, true
}

// PATCH /api/users/{id} {password?, disabled?}
func (s *Server) usersUpdate(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := userIDParam(w, r)
	if !ok {
		return
	}
	var req struct {
		Password *string `json:"password"`
		Disabled *bool   `json:"disabled"`
	}
	if err := decode(w, r, &req); err != nil || (req.Password == nil && req.Disabled == nil) {
		writeErr(w, 400, "Provide password or disabled")
		return
	}
	if claims, ok := s.requestClaims(r); ok && claims.Uid == id && req.Disabled != nil && *req.Disabled {
		writeErr(w, 400, "You cannot disable your own account")
		return
	}
	var hash []byte
	if req.Password != nil {
		if msg := validatePassword(*req.Password); msg != "" {
			writeErr(w, 400, msg)
			return
		}
		var err error
		if hash, err = bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost); err != nil {
			writeErr(w, 500, "Password encryption failed")
			return
		}
	}
	var u *db.User
	var err error
	if hash != nil {
		u, err = pg.SetUserPassword(id, string(hash))
	}
	if err == nil && req.Disabled != nil {
		u, err = pg.SetUserDisabled(id, *req.Disabled)
	}
	if errors.Is(err, db.ErrUserNotFound) {
		writeErr(w, 404, "User not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.users.put(u)
	writeJSON(w, 200, userView{User: u})
}

// DELETE /api/users/{id}
func (s *Server) usersDelete(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id, ok := userIDParam(w, r)
	if !ok {
		return
	}
	if claims, ok := s.requestClaims(r); ok && claims.Uid == id {
		writeErr(w, 400, "You cannot delete your own account")
		return
	}
	if err := pg.DeleteUser(id); errors.Is(err, db.ErrUserNotFound) {
		writeErr(w, 404, "User not found")
		return
	} else if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.users.remove(id)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/users/audit?limit=&username=
func (s *Server) auditList(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	entries, err := pg.ListAudit(limit, r.URL.Query().Get("username"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"entries": entries})
}

// audit records one row off the request path; a failure is only logged.
func (s *Server) audit(username, action, path string, status int, ip string) {
	if s.m == nil || s.m.pg == nil {
		return
	}
	pg := s.m.pg
	go func() {
		if err := pg.AddAudit(db.AuditEntry{Username: username, Action: action, Path: path, Status: status, IP: ip}); err != nil {
			log.Printf("[audit] %s %s %s: %v", username, action, path, err)
		}
	}()
}

// auditMutations records every state-changing request of an authenticated
// session: who, method, path (no query string, no body) and status.
func (s *Server) auditMutations(username string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			h.ServeHTTP(w, r)
			return
		}
		rec := &statusRecorder{ResponseWriter: w}
		h.ServeHTTP(rec, r)
		status := rec.status
		if status == 0 {
			status = 200
		}
		s.audit(username, r.Method, r.URL.Path, status, clientIP(r, s.trustedProxies))
	})
}

// statusRecorder captures the status code. It forwards Flush for streaming
// handlers and Unwrap for http.ResponseController.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = 200
	}
	return r.ResponseWriter.Write(b)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
