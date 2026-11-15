// Package app composes Creation's real stateful workspace/project source process.
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

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/serviceauth"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// Config contains only this Creation program's database, broker and directional
// private peers. It never loads shared role configuration or a business .env.
type Config struct {
	HTTPAddress         string                     `json:"http_address"`
	WriterAddress       string                     `json:"writer_address"`
	RegistrationAddress string                     `json:"registration_address"`
	PreparationAddress  string                     `json:"preparation_address"`
	GovernanceAddress   string                     `json:"governance_address"`
	PublicOrigin        string                     `json:"public_origin"`
	DatabaseDSN         string                     `json:"database_dsn"`
	KafkaBrokers        string                     `json:"kafka_brokers"`
	TraceEndpoint       string                     `json:"trace_endpoint"`
	TLS                 ServiceTLS                 `json:"tls"`
	IdentityAuthority   Peer                       `json:"identity_authority"`
	ExecutionBudget     Peer                       `json:"execution_budget"`
	ExecutionWork Peer `json:"execution_work"`
	MediaWork Peer `json:"media_work"`
	MediaCover Peer `json:"media_cover"`
	IdentityWriterToken string                     `json:"identity_writer_token"`
	RegistrationToken   string                     `json:"registration_token"`
	PreparationToken    string                     `json:"preparation_token"`
	GovernanceToken     string                     `json:"governance_token"`
	Writers             map[writefence.Writer]Peer `json:"writers"`
}

// ServiceTLS is the dedicated Creation certificate and private trust root.
type ServiceTLS struct {
	CertificatePEM string `json:"certificate_pem"`
	PrivateKeyPEM  string `json:"private_key_pem"`
	CAPEM          string `json:"ca_pem"`
	OwnName        string `json:"own_name"`
}

// Peer is deployment-fixed; no public URL or credential selects a remote target.
type Peer struct {
	Address    string `json:"address"`
	Token      string `json:"token"`
	ServerName string `json:"server_name"`
}

var errConfiguration = errors.New("complete Creation workspace source configuration required")

// LoadEnvironment reads only the explicitly scoped JSON value, without ENVFILE.
func LoadEnvironment(lookup func(string) (string, bool)) (Config, error) {
	if lookup == nil {
		return Config{}, errConfiguration
	}
	value, ok := lookup("GLOWLOOM_CREATION_WORKSPACE_CONFIG")
	if !ok || len(value) == 0 || len(value) > 65536 {
		return Config{}, errConfiguration
	}
	var cfg Config
	decoder := json.NewDecoder(bytes.NewBufferString(value))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cfg) != nil || decoder.Decode(new(any)) != io.EOF {
		return Config{}, errConfiguration
	}
	return cfg, cfg.Validate()
}

func validAddress(value string, ephemeral bool) bool {
	host, port, err := net.SplitHostPort(value)
	number, parseErr := strconv.Atoi(port)
	return err == nil && parseErr == nil && host != "" && !strings.ContainsAny(host, "/@?#% \t\r\n") && number >= 0 && number <= 65535 && (ephemeral || number > 0)
}

// Validate refuses incomplete peers, reused credentials and a noncanonical origin.
func (c Config) Validate() error {
	for _, address := range []string{c.HTTPAddress, c.WriterAddress, c.RegistrationAddress, c.PreparationAddress, c.GovernanceAddress} {
		if !validAddress(address, true) {
			return errConfiguration
		}
	}
	origin, err := url.Parse(c.PublicOrigin)
	if err != nil || origin.Host == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || origin.Opaque != "" || origin.ForceQuery {
		return errConfiguration
	}
	local := origin.Hostname() == "localhost" || net.ParseIP(origin.Hostname()).IsLoopback()
	if origin.Scheme != "https" && (origin.Scheme != "http" || !local) {
		return errConfiguration
	}
	if c.DatabaseDSN == "" || c.KafkaBrokers == "" || c.TLS.CertificatePEM == "" || c.TLS.PrivateKeyPEM == "" || c.TLS.CAPEM == "" || c.TLS.OwnName == "" || len(c.Writers) != len(writefence.Writers()) {
		return errConfiguration
	}
	seen := map[string]bool{}
	checkToken := func(token string) bool {
		if !serviceauth.ValidToken(token) || seen[token] {
			return false
		}
		seen[token] = true
		return true
	}
	for _, token := range []string{c.IdentityWriterToken, c.RegistrationToken, c.PreparationToken, c.GovernanceToken} {
		if !checkToken(token) {
			return errConfiguration
		}
	}
	for _, peer := range []Peer{c.IdentityAuthority, c.ExecutionBudget,c.ExecutionWork,c.MediaWork,c.MediaCover} {
		if !validAddress(peer.Address, false) || peer.ServerName == "" || !checkToken(peer.Token) {
			return errConfiguration
		}
	}
	for _, writer := range writefence.Writers() {
		peer, ok := c.Writers[writer]
		if !ok || peer.ServerName == "" || (writer != writefence.WriterCreation && !validAddress(peer.Address, false)) || !checkToken(peer.Token) {
			return errConfiguration
		}
	}
	return nil
}
