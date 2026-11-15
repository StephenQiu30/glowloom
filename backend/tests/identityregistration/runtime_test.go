package identityregistration_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	stdmail "net/mail"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	identityauthority "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/grpcauthority"
	identityhttp "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/http"
	identitymail "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/mail"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/identity/app"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

type capturedMail struct {
	recipient string
	message   []byte
}

func runtimeMailBody(t *testing.T, mail capturedMail) []byte {
	t.Helper()
	message, err := stdmail.ReadMessage(bytes.NewReader(mail.message))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := io.ReadAll(message.Body)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(encoded)), ""))
	if err != nil {
		t.Fatal(err)
	}
	return plain
}

func smtpSink(t *testing.T, certs *tlsFixture) (int, <-chan capturedMail) {
	t.Helper()
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certs.certificate(t, "localhost", x509.ExtKeyUsageServerAuth)}})
	if err != nil {
		t.Fatal(err)
	}
	messages := make(chan capturedMail, 256)
	var active sync.WaitGroup
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			active.Go(func() {
				defer func() { _ = conn.Close() }()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				reader := bufio.NewReader(conn)
				_, _ = io.WriteString(conn, "220 fixture SMTP\r\n")
				recipient := ""
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					switch {
					case strings.HasPrefix(line, "EHLO "), strings.HasPrefix(line, "HELO "):
						_, _ = io.WriteString(conn, "250 fixture\r\n")
					case strings.HasPrefix(line, "MAIL FROM:"):
						_, _ = io.WriteString(conn, "250 ok\r\n")
					case strings.HasPrefix(line, "RCPT TO:"):
						recipient = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "RCPT TO:")), "<>")
						_, _ = io.WriteString(conn, "250 ok\r\n")
					case strings.TrimSpace(line) == "DATA":
						_, _ = io.WriteString(conn, "354 continue\r\n")
						var body bytes.Buffer
						for body.Len() < 65536 {
							part, err := reader.ReadString('\n')
							if err != nil {
								return
							}
							if part == ".\r\n" {
								break
							}
							body.WriteString(part)
						}
						messages <- capturedMail{recipient: recipient, message: append([]byte(nil), body.Bytes()...)}
						_, _ = io.WriteString(conn, "250 accepted\r\n")
					case strings.TrimSpace(line) == "QUIT":
						_, _ = io.WriteString(conn, "221 bye\r\n")
						return
					default:
						_, _ = io.WriteString(conn, "250 ok\r\n")
					}
				}
			})
		}
	}()
	t.Cleanup(func() { _ = listener.Close(); <-done; active.Wait() })
	return listener.Addr().(*net.TCPAddr).Port, messages
}
func runtimeConfig(t *testing.T, f *registrationFixture) (identityapp.Config, <-chan capturedMail) {
	t.Helper()
	certificate := f.tls.certificate(t, "identity.registration", x509.ExtKeyUsageAny)
	private, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	ca := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.tls.authority.Raw}))
	// These are the same synthetic fixture keys used by its already accepted rows.
	key, err := json.Marshal(map[string]any{"key_id": "registration-fixture", "proof_key": []byte(strings.Repeat("h", 32)), "encryption_key": []byte(strings.Repeat("e", 32))})
	if err != nil {
		t.Fatal(err)
	}
	port, messages := smtpSink(t, f.tls)
	cfg := identityapp.Config{HTTPAddress: "127.0.0.1:0", AuthorityAddress: "127.0.0.1:0", WriterAddress: "127.0.0.1:0", PublicOrigin: "http://127.0.0.1", DatabaseDSN: "postgres://owner_b_identity_test@127.0.0.1:52080/" + os.Getenv("GLOWLOOM_TEST_REGISTRATION_DB_NAME") + "?sslmode=disable", IdentityKey: string(key), TermsVersion: identityhttp.LegalPackageVersion(), MailSourceHourlyLimit: 100000, MailGlobalHourlyLimit: 100000,
		Mail: identitymail.SMTPConfig{Host: "localhost", Port: port, TLSMode: "tls", FromEmail: "identity@fixture.invalid", FromName: "Identity fixture"}, MailCAPEM: ca,
		TLS:                  identityapp.ServiceTLS{CertificatePEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]})), PrivateKeyPEM: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})), CAPEM: ca, OwnName: "identity.registration"},
		CreationControlToken: strings.Repeat("creation-writer-", 3) + "identity", CreationControlPeerName: "creation.controls", Writers: map[writefence.Writer]identityapp.Peer{}, AuthorityTokens: map[writefence.Writer]string{}, AuthorityPeerNames: map[writefence.Writer]string{},
		CreationRegistration: identityapp.Peer{Address: f.creationPeer.Address, Token: f.creationPeer.Token, ServerName: f.creationPeer.Name}, ExecutionWallet: identityapp.Peer{Address: f.walletPeer.Address, Token: f.walletPeer.Token, ServerName: f.walletPeer.Name},
		// Tests that exercise team governance replace this closed peer with their
		// actual listener. Authentication-only tests must never call it.
		CreationGovernance: identityapp.Peer{Address: "127.0.0.1:1", Token: strings.Repeat("creation-governance-", 3), ServerName: "creation.governance"}}
	for _, writer := range writefence.Writers() {
		p := f.writerPeers[writer]
		cfg.Writers[writer] = identityapp.Peer{Address: p.Address, Token: p.Token, ServerName: p.Name}
		cfg.AuthorityTokens[writer] = strings.Repeat("authority-fixture-", 3) + string(writer)
		cfg.AuthorityPeerNames[writer] = string(writer) + ".authority"
	}
	cfg.AuthorityPeerNames[writefence.WriterIdentity] = cfg.TLS.OwnName
	return cfg, messages
}
func TestIdentityRuntimeOnlyLoadsOwnerConfiguration(t *testing.T) {
	_, err := identityapp.LoadEnvironment(func(key string) (string, bool) {
		if key != "GLOWLOOM_IDENTITY_CONFIG" {
			t.Fatalf("read unrelated configuration %s", key)
		}
		return `{"database_dsn":"must not connect", "foreign_owner_key":"rejected"}`, true
	})
	if err == nil {
		t.Fatal("accepted unknown or partial owner configuration")
	}
}

func TestIdentityRuntimeRealLoginMailAndRevocation(t *testing.T) {
	f := newRegistrationFixture(t)
	input := f.input(t)
	accountID, err := f.auth.CompleteRegistration(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	var email string
	if err := f.idb.Raw(`SELECT email_key FROM identity."user" WHERE id=?`, accountID).Scan(&email).Error; err != nil {
		t.Fatal(err)
	}
	mailEmail := uuid.NewString() + "@mail-worker.invalid"
	// A synthetic accepted application fixture supplies delivery work. The real
	// program must still enforce its prepared, unaccepted public legal package.
	if _, err := f.auth.StartRegistration(t.Context(), mailEmail, ""); err != nil {
		t.Fatal(err)
	}
	cfg, messages := runtimeConfig(t, f)
	app, err := identityapp.New(t.Context(), cfg, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(10 * time.Second):
			t.Error("Identity runtime did not join every worker/server")
		}
	})
	address, _, _ := app.Addresses()
	origin := "http://" + address
	client := &http.Client{Timeout: 15 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	send := func(method, path string, key uuid.UUID, body any, cookies []*http.Cookie, csrf string) *http.Response {
		var reader io.Reader
		if body != nil {
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			reader = bytes.NewReader(encoded)
		}
		request, err := http.NewRequestWithContext(t.Context(), method, origin+path, reader)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", cfg.PublicOrigin)
		request.Header.Set("Idempotency-Key", key.String())
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		if csrf != "" {
			request.Header.Set("X-CSRF-Token", csrf)
		}
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	ready := send(http.MethodGet, "/readyz", uuid.New(), nil, nil, "")
	_ = ready.Body.Close()
	if ready.StatusCode != 200 {
		t.Fatalf("runtime ready=%d", ready.StatusCode)
	}
	started := send(http.MethodPost, "/api/registrations", uuid.New(), identityhttp.RegistrationStartRequest{Email: uuid.NewString() + "@public-runtime.invalid"}, nil, "")
	_ = started.Body.Close()
	if identityhttp.AcceptedLegalVersion(cfg.TermsVersion) == "" && (started.StatusCode != 503 || len(started.Cookies()) != 0) {
		t.Fatalf("prepared legal package incorrectly enabled registration: %d", started.StatusCode)
	}
	var code string
	timeout := time.NewTimer(20 * time.Second)
	defer timeout.Stop()
	for code == "" {
		select {
		case <-timeout.C:
			t.Fatal("owned SMTP worker did not deliver accepted registration proof")
		case mail := <-messages:
			if mail.recipient != mailEmail {
				continue
			}
			message, err := stdmail.ReadMessage(bytes.NewReader(mail.message))
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := io.ReadAll(message.Body)
			if err != nil {
				t.Fatal(err)
			}
			plain, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(encoded)), ""))
			if err != nil {
				t.Fatal(err)
			}
			code = regexp.MustCompile(`[0-9]{6}`).FindString(string(plain))
		}
	}
	loginID := uuid.New()
	loggedIn := send(http.MethodPost, "/api/sessions", loginID, identityhttp.EmailLoginRequest{Email: email, Password: input.Password}, nil, "")
	if loggedIn.StatusCode != 201 {
		t.Fatalf("login status=%d", loggedIn.StatusCode)
	}
	var session map[string]any
	if err := json.NewDecoder(loggedIn.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	_ = loggedIn.Body.Close()
	cookies := loggedIn.Cookies()
	csrf, _ := session["csrf_token"].(string)
	if csrf == "" || len(cookies) == 0 {
		t.Fatal("native login omitted original cookies/CSRF contract")
	}
	me := send(http.MethodGet, "/api/me", uuid.New(), nil, cookies, "")
	if me.StatusCode != 200 {
		t.Fatalf("native profile read=%d", me.StatusCode)
	}
	var profile map[string]any
	if err := json.NewDecoder(me.Body).Decode(&profile); err != nil {
		t.Fatal(err)
	}
	_ = me.Body.Close()
	for _, secret := range []string{"password_hash", "session_epoch", "token_hash", "refresh_token"} {
		if _, exists := profile[secret]; exists {
			t.Fatalf("profile exposed %s", secret)
		}
	}
	preferences := send(http.MethodGet, "/api/me/preferences", uuid.New(), nil, cookies, "")
	if preferences.StatusCode != 200 {
		t.Fatalf("native account preferences=%d", preferences.StatusCode)
	}
	var originalPreferences identityhttp.PreferencesView
	if err := json.NewDecoder(preferences.Body).Decode(&originalPreferences); err != nil {
		t.Fatal(err)
	}
	_ = preferences.Body.Close()
	theme := "dark"
	patch := identityhttp.PreferencesUpdateRequest{ExpectedRevision: originalPreferences.Revision, Theme: &theme}
	updated := send(http.MethodPatch, "/api/me/preferences", uuid.New(), patch, cookies, csrf)
	if updated.StatusCode != 200 {
		t.Fatalf("native preference mutation=%d", updated.StatusCode)
	}
	_ = updated.Body.Close()
	var activityBeforeConflict, activityAfterConflict time.Time
	if err := f.idb.Raw(`SELECT s.last_active_at FROM identity.user_session s JOIN identity.session_activation a ON a.session_id=s.id WHERE a.request_id=?`, loginID).Scan(&activityBeforeConflict).Error; err != nil {
		t.Fatal(err)
	}
	stalePreferences := send(http.MethodPatch, "/api/me/preferences", uuid.New(), patch, cookies, csrf)
	_ = stalePreferences.Body.Close()
	if stalePreferences.StatusCode != 409 {
		t.Fatalf("stale account preference revision=%d", stalePreferences.StatusCode)
	}
	if err := f.idb.Raw(`SELECT s.last_active_at FROM identity.user_session s JOIN identity.session_activation a ON a.session_id=s.id WHERE a.request_id=?`, loginID).Scan(&activityAfterConflict).Error; err != nil || !activityAfterConflict.Equal(activityBeforeConflict) {
		t.Fatal("failed preference write counted as successful foreground activity")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.security_event WHERE target_user_id=? AND action='preferences.updated'`, accountID) != 1 {
		t.Fatal("native preference update repeated its audit event")
	}
	var epoch int64
	if err := f.idb.Raw(`SELECT session_epoch FROM identity."user" WHERE id=?`, accountID).Scan(&epoch).Error; err != nil {
		t.Fatal(err)
	}
	passwordKey := uuid.New()
	newPassword := "runtime-next-fixture-password"
	passwordBody := identityhttp.EmailPasswordChangeRequest{Epoch: strconv.FormatInt(epoch, 10), Current: input.Password, New: newPassword, Confirmation: newPassword}
	changed := send(http.MethodPost, "/api/me/password", passwordKey, passwordBody, cookies, csrf)
	if changed.StatusCode != 200 {
		t.Fatalf("native password qualification status=%d", changed.StatusCode)
	}
	var changedView map[string]any
	if err := json.NewDecoder(changed.Body).Decode(&changedView); err != nil {
		t.Fatal(err)
	}
	_ = changed.Body.Close()
	newCSRF, _ := changedView["csrf_token"].(string)
	if newCSRF == "" || newCSRF == csrf {
		t.Fatal("password command did not return its original replacement CSRF")
	}
	repeated := send(http.MethodPost, "/api/me/password", passwordKey, passwordBody, cookies, csrf)
	if repeated.StatusCode != 200 {
		t.Fatalf("password original revoked-cookie recovery=%d", repeated.StatusCode)
	}
	var repeatedView map[string]any
	if err := json.NewDecoder(repeated.Body).Decode(&repeatedView); err != nil {
		t.Fatal(err)
	}
	_ = repeated.Body.Close()
	if repeatedView["csrf_token"] != newCSRF {
		t.Fatal("password retry minted another reply")
	}
	responsibility, err := pgidentity.NewPasswordChangeStore(f.idb).LoadAcceptedPasswordChange(t.Context(), passwordKey)
	if err != nil || !responsibility.Completed {
		t.Fatalf("password qualification responsibility not complete: %v", err)
	}
	for _, control := range []writefence.Control{responsibility.CloseControl, responsibility.ReopenControl, responsibility.SessionControl} {
		state, err := pgfence.NewAuthorityStore(f.idb).LoadCoordination(t.Context(), control.CoordinationID)
		if err != nil || !state.Complete || state.Control != control || len(state.Acknowledged) != 4 {
			t.Fatalf("password qualification success before exact four ACKs: %v", err)
		}
	}
	cookies = changed.Cookies()
	csrf = newCSRF
	_, authorityAddress, _ := app.Addresses()
	authorityClient, err := identityauthority.NewClient(writefence.WriterIdentity, identityauthority.Target{Address: authorityAddress, Token: cfg.AuthorityTokens[writefence.WriterIdentity], Transport: f.tls.client(t, cfg.TLS.OwnName, cfg.TLS.OwnName)})
	if err != nil {
		t.Fatal(err)
	}
	readAuthorities := func(cookies []*http.Cookie, credentialEpoch, accountEpoch int64) {
		t.Helper()
		var token string
		for _, cookie := range cookies {
			if cookie.Name == "glowloom_session" {
				token = cookie.Value
			}
		}
		state, err := authorityClient.ReadSessionAuthorities(t.Context(), identityauthority.LegacyCredential(token, ""))
		if err != nil || state.Identity.AccountID != accountID || state.Identity.CredentialEpoch != credentialEpoch || state.Account.Epoch != accountEpoch || state.Session.Epoch != 1 || state.Account.ID != accountID || state.Session.ID != state.Identity.SessionID {
			t.Fatalf("native mTLS authority snapshot=%v %v", state, err)
		}
	}
	readAuthorities(cookies, 2, 3)
	var token string
	for _, cookie := range cookies {
		if cookie.Name == "glowloom_session" {
			token = cookie.Value
		}
	}
	var beforeActivity struct{ LastActiveAt, AbsoluteExpiresAt time.Time }
	if err := f.idb.Raw(`SELECT last_active_at,absolute_expires_at FROM identity.user_session WHERE id=?`, responsibility.ReplacementSessionID).Scan(&beforeActivity).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := authorityClient.Verify(t.Context(), identityauthority.LegacyCredential(token, "")); err != nil {
		t.Fatal(err)
	}
	readAuthorities(cookies, 2, 3)
	var unchangedActivity struct{ LastActiveAt time.Time }
	if err := f.idb.Raw(`SELECT last_active_at FROM identity.user_session WHERE id=?`, responsibility.ReplacementSessionID).Scan(&unchangedActivity).Error; err != nil || !unchangedActivity.LastActiveAt.Equal(beforeActivity.LastActiveAt) {
		t.Fatal("private verification or authority read touched activity")
	}
	if _, err := authorityClient.RecordForegroundActivity(t.Context(), identityauthority.LegacyCredential(token, ""), identity.ForegroundProfileWrite); err == nil {
		t.Fatal("native foreground write accepted missing CSRF")
	}
	if _, err := authorityClient.RecordForegroundActivity(t.Context(), identityauthority.LegacyCredential(token, ""), identity.ForegroundProfileRead); err != nil {
		t.Fatal(err)
	}
	var afterActivity struct{ LastActiveAt, AbsoluteExpiresAt time.Time }
	if err := f.idb.Raw(`SELECT last_active_at,absolute_expires_at FROM identity.user_session WHERE id=?`, responsibility.ReplacementSessionID).Scan(&afterActivity).Error; err != nil || !afterActivity.LastActiveAt.After(beforeActivity.LastActiveAt) || !afterActivity.AbsoluteExpiresAt.Equal(beforeActivity.AbsoluteExpiresAt) {
		t.Fatal("explicit native foreground activity failed to update only idle time")
	}
	var sessionRow struct{ ID uuid.UUID }
	if err := f.idb.Raw(`SELECT id FROM identity.user_session WHERE account_id=? AND revoked_at IS NULL`, accountID).Scan(&sessionRow).Error; err != nil || sessionRow.ID == uuid.Nil {
		t.Fatalf("native session lookup failed: %v", err)
	}
	sessionID := sessionRow.ID
	revoked := send(http.MethodDelete, fmt.Sprintf("/api/sessions/%s", sessionID), uuid.New(), nil, cookies, csrf)
	_ = revoked.Body.Close()
	if revoked.StatusCode != 204 {
		t.Fatalf("durable revocation status=%d", revoked.StatusCode)
	}
	closure, err := pgidentity.NewSessionRevocationStore(f.idb).LoadSessionRevocation(t.Context(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := pgfence.NewAuthorityStore(f.idb).LoadCoordination(t.Context(), closure.Control.CoordinationID)
	if err != nil || !state.Complete || state.Control != closure.Control || len(state.Acknowledged) != 4 {
		t.Fatalf("native revocation returned before exact four writer ACKs: %v", err)
	}
	current := send(http.MethodGet, "/api/session", uuid.New(), nil, cookies, "")
	_ = current.Body.Close()
	if current.StatusCode != 401 {
		t.Fatalf("revoked cookie remained usable: %d", current.StatusCode)
	}
	// Reset is anonymous: exercise the real public start, owned TLS mail worker,
	// original-proof completion and replay without supplying a session cookie.
	bucket := hex.EncodeToString(f.protector.Digest("email_budget", uuid.Nil, email))
	if err := f.idb.Exec(`UPDATE identity.email_budget SET sent_at=ARRAY[statement_timestamp()-interval '61 seconds'] WHERE bucket_key=?`, bucket).Error; err != nil {
		t.Fatal(err)
	}
	resetStarted := send(http.MethodPost, "/api/password-resets", uuid.New(), identityhttp.PasswordResetStartRequest{Email: email}, nil, "")
	_ = resetStarted.Body.Close()
	if resetStarted.StatusCode != 202 || len(resetStarted.Cookies()) != 0 {
		t.Fatalf("anonymous reset start=%d", resetStarted.StatusCode)
	}
	var resetToken string
	resetDeadline := time.NewTimer(20 * time.Second)
	defer resetDeadline.Stop()
	for resetToken == "" {
		select {
		case <-resetDeadline.C:
			t.Fatal("owned TLS SMTP worker did not deliver reset proof")
		case mail := <-messages:
			if mail.recipient != email {
				continue
			}
			message, err := stdmail.ReadMessage(bytes.NewReader(mail.message))
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := io.ReadAll(message.Body)
			if err != nil {
				t.Fatal(err)
			}
			plain, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(encoded)), ""))
			if err != nil {
				t.Fatal(err)
			}
			match := regexp.MustCompile(`/reset-password#token=([^\s]+)`).FindSubmatch(plain)
			if len(match) == 2 {
				resetToken, err = url.QueryUnescape(string(match[1]))
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	resetKey := uuid.New()
	resetBody := identityhttp.PasswordResetCompleteRequest{Token: resetToken, Password: "runtime-reset-next-password", Confirmation: "runtime-reset-next-password"}
	for range 2 {
		reset := send(http.MethodPost, "/api/password-resets/complete", resetKey, resetBody, nil, "")
		_ = reset.Body.Close()
		if reset.StatusCode != 204 || len(reset.Cookies()) != 0 {
			t.Fatalf("anonymous original reset completion=%d cookies=%d", reset.StatusCode, len(reset.Cookies()))
		}
	}
	resetResponsibility, err := pgidentity.NewPasswordResetStore(f.idb).LoadAcceptedPasswordReset(t.Context(), resetKey)
	if err != nil || !resetResponsibility.Completed {
		t.Fatalf("reset source responsibility not complete: %v", err)
	}
	for _, control := range []writefence.Control{resetResponsibility.CloseControl, resetResponsibility.ReopenControl} {
		state, err := pgfence.NewAuthorityStore(f.idb).LoadCoordination(t.Context(), control.CoordinationID)
		if err != nil || !state.Complete || state.Control != control || len(state.Acknowledged) != 4 {
			t.Fatalf("reset success before exact four writer ACKs: %v", err)
		}
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.user_session WHERE account_id=? AND revoked_at IS NULL`, accountID) != 0 {
		t.Fatal("anonymous HTTP reset minted a session")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.security_event WHERE target_user_id=? AND action='password.changed'`, accountID) != 2 {
		t.Fatal("HTTP reset replay repeated password mutation")
	}
	oldLogin := send(http.MethodPost, "/api/sessions", uuid.New(), identityhttp.EmailLoginRequest{Email: email, Password: newPassword}, nil, "")
	_ = oldLogin.Body.Close()
	if oldLogin.StatusCode != 401 {
		t.Fatalf("pre-reset password login=%d", oldLogin.StatusCode)
	}
	resetLogin := send(http.MethodPost, "/api/sessions", uuid.New(), identityhttp.EmailLoginRequest{Email: email, Password: resetBody.Password}, nil, "")
	if resetLogin.StatusCode != 201 || len(resetLogin.Cookies()) == 0 {
		t.Fatalf("reset account cannot create its next session=%d", resetLogin.StatusCode)
	}
	var resetSession map[string]any
	if err := json.NewDecoder(resetLogin.Body).Decode(&resetSession); err != nil {
		t.Fatal(err)
	}
	_ = resetLogin.Body.Close()
	cookies = resetLogin.Cookies()
	csrf, _ = resetSession["csrf_token"].(string)
	reauthenticated := send(http.MethodPost, "/api/me/reauthentication", uuid.New(), identityhttp.ReauthenticationRequest{Password: resetBody.Password}, cookies, csrf)
	_ = reauthenticated.Body.Close()
	if reauthenticated.StatusCode != 204 {
		t.Fatalf("email recent authentication=%d", reauthenticated.StatusCode)
	}
	if err := f.idb.Exec(`UPDATE identity.email_budget SET sent_at=ARRAY[statement_timestamp()-interval '61 seconds'] WHERE bucket_key=?`, bucket).Error; err != nil {
		t.Fatal(err)
	}
	var revision int64
	if err := f.idb.Raw(`SELECT revision FROM identity."user" WHERE id=?`, accountID).Scan(&revision).Error; err != nil {
		t.Fatal(err)
	}
	nextEmail := uuid.NewString() + "@runtime-email.invalid"
	emailStart := send(http.MethodPost, "/api/me/email-change", uuid.New(), identityhttp.EmailChangeStartRequest{ExpectedRevision: strconv.FormatInt(revision, 10), NewEmail: nextEmail}, cookies, csrf)
	if emailStart.StatusCode != 202 {
		t.Fatalf("native email change start=%d", emailStart.StatusCode)
	}
	var emailOperation identityhttp.AcceptedView
	if err := json.NewDecoder(emailStart.Body).Decode(&emailOperation); err != nil {
		t.Fatal(err)
	}
	_ = emailStart.Body.Close()
	mailCodes := map[string]string{}
	emailDeadline := time.NewTimer(20 * time.Second)
	defer emailDeadline.Stop()
	for len(mailCodes) < 2 {
		select {
		case <-emailDeadline.C:
			t.Fatal("native mail worker did not deliver both email-change proofs")
		case mail := <-messages:
			side := "old"
			if mail.recipient == nextEmail {
				side = "new"
			} else if mail.recipient != email {
				continue
			}
			if code := regexp.MustCompile(`[0-9]{6}`).Find(runtimeMailBody(t, mail)); len(code) == 6 {
				mailCodes[side] = string(code)
			}
		}
	}
	for _, side := range []string{"old", "new"} {
		verified := send(http.MethodPost, "/api/me/email-change/"+emailOperation.OperationID.String()+"/verify", uuid.New(), identityhttp.EmailChangeVerifyRequest{Side: side, Code: mailCodes[side]}, cookies, csrf)
		_ = verified.Body.Close()
		if verified.StatusCode != 204 {
			t.Fatalf("native %s mailbox proof=%d", side, verified.StatusCode)
		}
	}
	emailKey := uuid.New()
	for range 2 {
		completed := send(http.MethodPost, "/api/me/email-change/"+emailOperation.OperationID.String()+"/complete", emailKey, identityhttp.EmailChangeCompleteRequest{ExpectedRevision: strconv.FormatInt(revision, 10)}, cookies, csrf)
		_ = completed.Body.Close()
		if completed.StatusCode != 204 {
			t.Fatalf("native original email completion after revoked-cookie=%d", completed.StatusCode)
		}
	}
	emailResponsibility, err := pgidentity.NewSourceEmailChangeStore(f.idb).LoadAcceptedEmailChange(t.Context(), emailKey)
	if err != nil || !emailResponsibility.Completed {
		t.Fatalf("native email responsibility not complete: %v", err)
	}
	for _, control := range []writefence.Control{emailResponsibility.CloseControl, emailResponsibility.ReopenControl} {
		state, err := pgfence.NewAuthorityStore(f.idb).LoadCoordination(t.Context(), control.CoordinationID)
		if err != nil || !state.Complete || state.Control != control || len(state.Acknowledged) != 4 {
			t.Fatal("native email success preceded exact four writer ACKs")
		}
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.security_event WHERE target_user_id=? AND action='email.changed'`, accountID) != 1 {
		t.Fatal("native email replay repeated audit/mutation")
	}
	if count(t, f.idb, `SELECT count(*) FROM identity.user_session WHERE account_id=? AND revoked_at IS NULL`, accountID) != 0 {
		t.Fatal("email change minted a replacement session")
	}
	mailboxLogin := send(http.MethodPost, "/api/sessions", uuid.New(), identityhttp.EmailLoginRequest{Email: nextEmail, Password: resetBody.Password}, nil, "")
	if mailboxLogin.StatusCode != 201 {
		t.Fatalf("changed mailbox cannot login=%d", mailboxLogin.StatusCode)
	}
	var mailboxSession map[string]any
	if err := json.NewDecoder(mailboxLogin.Body).Decode(&mailboxSession); err != nil {
		t.Fatal(err)
	}
	_ = mailboxLogin.Body.Close()
	cookies = mailboxLogin.Cookies()
	csrf, _ = mailboxSession["csrf_token"].(string)
	readAuthorities(cookies, 4, 7)
	// Synthetic fixture promotion supplies current platform authority; the public
	// actions below still require real recent authentication and preserve the last
	// available administrator. No platform grant endpoint is implied by the fixture.
	f.clock = time.Now().UTC().Truncate(time.Microsecond)
	targetRegistration := f.input(t)
	targetID, err := f.auth.CompleteRegistration(t.Context(), targetRegistration)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{accountID, targetID} {
		if err := f.idb.Exec(`UPDATE identity."user" SET platform_role='platform_admin',platform_epoch=platform_epoch+1 WHERE id=?`, id).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		if err := f.idb.Exec(`UPDATE identity."user" SET platform_role='creator' WHERE id IN ?`, []uuid.UUID{accountID, targetID}).Error; err != nil {
			t.Error(err)
		}
	})
	platformReauth := send(http.MethodPost, "/api/me/reauthentication", uuid.New(), identityhttp.ReauthenticationRequest{Password: resetBody.Password}, cookies, csrf)
	_ = platformReauth.Body.Close()
	if platformReauth.StatusCode != 204 {
		t.Fatalf("native platform recent proof=%d", platformReauth.StatusCode)
	}
	for _, step := range []struct{ path, action string }{{"revoke-sessions", "revoke_sessions"}, {"revoke-admin", "revoke_admin"}} {
		var version int64
		if err := f.idb.Raw(`SELECT revision FROM identity."user" WHERE id=?`, targetID).Scan(&version).Error; err != nil {
			t.Fatal(err)
		}
		key := uuid.New()
		for range 2 {
			response := send(http.MethodPost, "/api/platform/users/"+targetID.String()+"/"+step.path, key, identityhttp.GovernanceMutation{ExpectedRevision: version, Reason: "synthetic platform security action"}, cookies, csrf)
			_ = response.Body.Close()
			if response.StatusCode != 204 {
				t.Fatalf("native %s source command=%d", step.action, response.StatusCode)
			}
		}
		p, err := pgidentity.NewSourceGovernanceStore(f.idb).LoadAcceptedGovernance(t.Context(), key)
		if err != nil || !p.Completed {
			t.Fatalf("native %s responsibility not completed: %v", step.action, err)
		}
		for _, control := range []writefence.Control{p.CloseControl, p.ReopenControl} {
			state, err := pgfence.NewAuthorityStore(f.idb).LoadCoordination(t.Context(), control.CoordinationID)
			if err != nil || !state.Complete || state.Control != control || len(state.Acknowledged) != 4 {
				t.Fatalf("native %s success before four actual ACKs", step.action)
			}
		}
		if count(t, f.idb, `SELECT count(*) FROM identity.security_event WHERE target_user_id=? AND action=?`, targetID, "account."+step.action) != 1 {
			t.Fatal("native governance replay repeated local mutation")
		}
	}
	var ownVersion int64
	if err := f.idb.Raw(`SELECT revision FROM identity."user" WHERE id=?`, accountID).Scan(&ownVersion).Error; err != nil {
		t.Fatal(err)
	}
	lastAdmin := send(http.MethodPost, "/api/platform/users/"+accountID.String()+"/revoke-admin", uuid.New(), identityhttp.GovernanceMutation{ExpectedRevision: ownVersion, Reason: "synthetic last administrator check"}, cookies, csrf)
	_ = lastAdmin.Body.Close()
	if lastAdmin.StatusCode != 409 {
		t.Fatalf("native last platform administrator protection=%d", lastAdmin.StatusCode)
	}
}
