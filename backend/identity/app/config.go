// Package app composes Identity's independently owned authentication process.
package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	identitymail "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/mail"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// Config is an owner-only configuration. Loading it never reads .env, a legacy
// role, another owner's database or storage/provider credentials.
type Config struct {
	HTTPAddress             string                       `json:"http_address"`
	AuthorityAddress        string                       `json:"authority_address"`
	WriterAddress           string                       `json:"writer_address"`
	PublicOrigin            string                       `json:"public_origin"`
	DatabaseDSN             string                       `json:"database_dsn"`
	IdentityKey             string                       `json:"identity_key"`
	TermsVersion            string                       `json:"terms_version"`
	MailSourceHourlyLimit   int                          `json:"mail_source_hourly_limit"`
	MailGlobalHourlyLimit   int                          `json:"mail_global_hourly_limit"`
	Mail                    identitymail.SMTPConfig      `json:"mail"`
	MailCAPEM               string                       `json:"mail_ca_pem"`
	TLS                     ServiceTLS                   `json:"tls"`
	AuthorityTokens         map[writefence.Writer]string `json:"authority_tokens"`
	AuthorityPeerNames      map[writefence.Writer]string `json:"authority_peer_names"`
	CreationControlToken    string                       `json:"creation_control_token"`
	CreationControlPeerName string                       `json:"creation_control_peer_name"`
	Writers                 map[writefence.Writer]Peer   `json:"writers"`
	CreationRegistration    Peer                         `json:"creation_registration"`
	CreationGovernance      Peer                         `json:"creation_governance"`
	ExecutionWallet         Peer                         `json:"execution_wallet"`
	Session                 SessionConfig                `json:"session"`
	TraceEndpoint           string                       `json:"trace_endpoint"`
}

// ServiceTLS is the dedicated Identity service identity and private trust root.
type ServiceTLS struct{ CertificatePEM, PrivateKeyPEM, CAPEM, OwnName string }

// Peer is a deployment-fixed audience, never a URL supplied by a public request.
type Peer struct{ Address, Token, ServerName string }

// SessionConfig contains only Identity's JWT/refresh security dependencies.
type SessionConfig struct {
	Enabled                                       bool
	PrivateKey, VerifyKeys, ActiveKID, ReceiptKey string
	RedisAddress, RedisUsername, RedisPassword    string
	RedisDB                                       int
	LegacyUpgradeUntil                            time.Time
}

var errConfiguration = errors.New("complete Identity owner configuration required")

// LoadEnvironment reads exactly one explicitly scoped value and never uses a
// global configuration loader or follows ENVFILE. Errors contain no private data.
func LoadEnvironment(lookup func(string) (string, bool)) (Config, error) {
	if lookup == nil {
		return Config{}, errConfiguration
	}
	value, ok := lookup("GLOWLOOM_IDENTITY_CONFIG")
	if !ok || len(value) == 0 || len(value) > 65536 {
		return Config{}, errConfiguration
	}
	var cfg Config
	decoder := json.NewDecoder(bytes.NewBufferString(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, errConfiguration
	}
	if decoder.Decode(new(any)) != io.EOF {
		return Config{}, errConfiguration
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
func validAddress(address string, ephemeral bool) bool {
	host, port, err := net.SplitHostPort(address)
	n, parseErr := strconv.Atoi(port)
	return err == nil && parseErr == nil && host != "" && !strings.ContainsAny(host, "/@?#% \t\n\r") && n >= 0 && n <= 65535 && (ephemeral || n != 0)
}

// Validate closes the dependency set before a listener or worker is exposed.
func (c Config) Validate() error {
	if !validAddress(c.HTTPAddress, true) || !validAddress(c.AuthorityAddress, true) || !validAddress(c.WriterAddress, true) || c.DatabaseDSN == "" || c.IdentityKey == "" || c.MailSourceHourlyLimit < 1 || c.MailGlobalHourlyLimit < 1 {
		return errConfiguration
	}
	origin, err := url.Parse(c.PublicOrigin)
	if err != nil || origin.User != nil || origin.Host == "" || origin.RawQuery != "" || origin.Fragment != "" || (origin.Path != "" && origin.Path != "/") {
		return errConfiguration
	}
	local := origin.Hostname() == "localhost" || net.ParseIP(origin.Hostname()).IsLoopback()
	if origin.Scheme != "https" && (origin.Scheme != "http" || !local) {
		return errConfiguration
	}
	if c.TLS.CertificatePEM == "" || c.TLS.PrivateKeyPEM == "" || c.TLS.CAPEM == "" || c.TLS.OwnName == "" || !serviceauth.ValidToken(c.CreationControlToken) || c.CreationControlPeerName == "" {
		return errConfiguration
	}
	if len(c.Writers) != 4 || len(c.AuthorityTokens) != 4 || len(c.AuthorityPeerNames) != 4 {
		return errConfiguration
	}
	seenTokens := map[string]bool{}
	seenNames := map[string]bool{}
	for _, writer := range writefence.Writers() {
		p, ok := c.Writers[writer]
		if !ok || p.ServerName == "" || !serviceauth.ValidToken(p.Token) || (writer != writefence.WriterIdentity && !validAddress(p.Address, false)) || seenTokens[p.Token] {
			return errConfiguration
		}
		seenTokens[p.Token] = true
		token, name := c.AuthorityTokens[writer], c.AuthorityPeerNames[writer]
		if !serviceauth.ValidToken(token) || seenTokens[token] || name == "" || seenNames[name] {
			return errConfiguration
		}
		seenTokens[token] = true
		seenNames[name] = true
	}
	for _, p := range []Peer{c.CreationRegistration, c.CreationGovernance, c.ExecutionWallet} {
		if !validAddress(p.Address, false) || p.ServerName == "" || !serviceauth.ValidToken(p.Token) || seenTokens[p.Token] {
			return errConfiguration
		}
		seenTokens[p.Token] = true
	}
	if seenTokens[c.CreationControlToken] {
		return errConfiguration
	}
	if c.Session.Enabled && (c.Session.PrivateKey == "" || c.Session.VerifyKeys == "" || c.Session.ActiveKID == "" || c.Session.ReceiptKey == "" || !validAddress(c.Session.RedisAddress, false)) {
		return errConfiguration
	}
	if c.Session.RedisDB < 0 {
		return errConfiguration
	}
	return nil
}
