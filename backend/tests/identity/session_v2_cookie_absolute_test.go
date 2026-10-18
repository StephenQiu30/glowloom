package identity_test

import (
	"net/http"
	"testing"
	"time"
)

// Recorder with an injected application clock, not deployed browser evidence.
func TestSessionV2RecoveredCookiesKeepAbsoluteDeadline(t *testing.T) {
	r, _, now := sessionHTTPFixture(t, false)
	login := sessionHTTPRequest(t, r, "POST", "/api/sessions", nil, "")
	if login.Code != http.StatusCreated {
		t.Fatal("synthetic login failed")
	}
	old := login.Result().Cookies()
	var csrf string
	for _, c := range old {
		if !c.HttpOnly {
			csrf = c.Value
		}
	}
	first := sessionHTTPRequest(t, r, "POST", "/api/session/refresh", old, csrf)
	if first.Code != http.StatusOK {
		t.Fatal("initial rotation failed")
	}
	*now = now.Add(9 * time.Second)
	recovered := sessionHTTPRequest(t, r, "POST", "/api/session/refresh", old, csrf)
	if recovered.Code != http.StatusOK {
		t.Fatal("receipt recovery failed")
	}
	original := first.Result().Cookies()
	cookies := recovered.Result().Cookies()
	if len(original) != 3 || len(cookies) != 3 {
		t.Fatal("missing credential cookies")
	}
	for i, c := range cookies {
		if c.Value != original[i].Value || !c.Expires.Equal(original[i].Expires) {
			t.Fatal("recovery changed child or absolute expiration")
		}
		// Max-Age overrides Expires and is relative to receipt by the browser.
		if c.MaxAge != 0 {
			t.Fatal("relative Max-Age can extend recovered cookie retention")
		}
	}
}
