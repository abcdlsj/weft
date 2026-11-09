package weft

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newAuthServer(t *testing.T) (*Server, *Store) {
	t.Helper()
	st, err := OpenStore(t.TempDir() + "/weft.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := SetPassword(st, "secret1"); err != nil {
		t.Fatal(err)
	}
	return NewServer(st, false), st
}

func login(t *testing.T, s *Server) (*http.Cookie, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"password":"secret1"}`))
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		CSRF string `json:"csrf"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.CSRF == "" {
		t.Fatalf("login did not return csrf: %s", rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("login did not set a session cookie")
	}
	return cookies[0], body.CSRF
}

func TestPasswordHashIsArgon2AndLegacyUpgrades(t *testing.T) {
	s, st := newAuthServer(t)
	hash, err := st.GetSetting("password_hash")
	if err != nil || !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("new password is not argon2id: %q (%v)", hash, err)
	}
	if !s.checkPassword("secret1") || s.checkPassword("wrong") {
		t.Fatal("argon2 verification failed")
	}

	// Simulate a database created by weft before v0.4.0.
	salt := "00112233445566778899aabbccddeeff"
	legacy, err := legacyPasswordHash("old-secret", salt)
	if err != nil {
		t.Fatal(err)
	}
	st.SetSetting("password_salt", salt)
	st.SetSetting("password_hash", legacy)
	if !s.checkPassword("old-secret") {
		t.Fatal("legacy pbkdf2 password no longer verifies")
	}
	upgraded, _ := st.GetSetting("password_hash")
	if !strings.HasPrefix(upgraded, "$argon2id$") {
		t.Fatalf("legacy hash was not upgraded: %q", upgraded)
	}
}

func TestAuthedMutationsRequireCSRF(t *testing.T) {
	s, _ := newAuthServer(t)
	cookie, csrf := login(t, s)

	do := func(token string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"defaultTemplate":"mixed-port: 7890"}`))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		if token != "" {
			req.Header.Set("X-CSRF-Token", token)
		}
		s.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	if code := do(""); code != http.StatusForbidden {
		t.Fatalf("missing csrf: got %d", code)
	}
	if code := do("wrong"); code != http.StatusForbidden {
		t.Fatalf("wrong csrf: got %d", code)
	}
	if code := do(csrf); code != http.StatusOK {
		t.Fatalf("right csrf: got %d", code)
	}
}

func TestSessionSurvivesServerRestart(t *testing.T) {
	s, st := newAuthServer(t)
	cookie, csrf := login(t, s)
	// A new Server instance over the same database is what a restart looks like.
	restarted := NewServer(st, false)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(cookie)
	restarted.Handler().ServeHTTP(rec, req)
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body["authenticated"] != true || body["csrf"] != csrf {
		t.Fatalf("session did not survive restart: %v", body)
	}
}

func TestSecurityHeaders(t *testing.T) {
	s, _ := newAuthServer(t)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/me", nil))
	h := rec.Header()
	for name, want := range map[string]string{
		"X-Frame-Options":        "DENY",
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "no-referrer",
	} {
		if h.Get(name) != want {
			t.Errorf("%s = %q, want %q", name, h.Get(name), want)
		}
	}
	if !strings.Contains(h.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Errorf("CSP missing frame-ancestors: %q", h.Get("Content-Security-Policy"))
	}
}

func TestRateLimiter(t *testing.T) {
	l := newRateLimiter(2, time.Minute)
	if !l.allow("a") || !l.allow("a") {
		t.Fatal("first two calls should pass")
	}
	if l.allow("a") {
		t.Fatal("third call should be limited")
	}
	if !l.allow("b") {
		t.Fatal("another key should have its own bucket")
	}
}
