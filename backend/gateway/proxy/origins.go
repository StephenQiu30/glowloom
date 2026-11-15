package proxy

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Origins names the four independent business HTTP owners. A partial set is
// never combined with the pre-migration backend or used as a fallback.
type Origins struct {
	Identity  string
	Creation  string
	Media     string
	Execution string
}

// Configured reports whether any domain origin was explicitly supplied.
func (o Origins) Configured() bool {
	return o.Identity != "" || o.Creation != "" || o.Media != "" || o.Execution != ""
}

func parseOrigin(origin string) (*url.URL, error) {
	target, err := url.Parse(origin)
	if err != nil || target == nil || (target.Scheme != "http" && target.Scheme != "https") ||
		target.Hostname() == "" || target.User != nil || target.Opaque != "" ||
		(target.Path != "" && target.Path != "/") || target.RawQuery != "" || target.ForceQuery || target.Fragment != "" {
		return nil, errors.New("gateway requires an HTTP(S) origin without credentials, path, query or fragment")
	}
	if strings.HasSuffix(target.Host, ":") {
		return nil, errors.New("gateway origin requires a valid port")
	}
	if port := target.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return nil, errors.New("gateway origin requires a valid port")
		}
	}
	target.Path = ""
	return target, nil
}

func (o Origins) targets() (map[string]*url.URL, error) {
	targets := make(map[string]*url.URL, 4)
	seen := make(map[string]bool, 4)
	for owner, origin := range map[string]string{"identity": o.Identity, "creation": o.Creation, "media": o.Media, "execution": o.Execution} {
		target, err := parseOrigin(origin)
		if err != nil {
			return nil, errors.New("gateway requires all four valid domain origins")
		}
		key := target.Scheme + "://" + net.JoinHostPort(canonicalHost(target.Hostname()), originPort(target))
		if seen[key] {
			return nil, errors.New("gateway domain origins must be distinct")
		}
		seen[key] = true
		targets[owner] = target
	}
	return targets, nil
}

// ValidateOrigins rejects incomplete, shared or directly self-referencing
// origins before opening the listener. Readiness separately guards probe cycles.
func ValidateOrigins(origins Origins, listener string) error {
	targets, err := origins.targets()
	if err != nil {
		return err
	}
	for _, target := range targets {
		if loopsToListener(target, listener) {
			return errors.New("gateway domain origin must not point to its listener")
		}
	}
	return nil
}

// ValidateLegacyOrigin validates the explicitly configured pre-migration origin.
func ValidateLegacyOrigin(origin, listener string) error {
	target, err := parseOrigin(origin)
	if err != nil {
		return err
	}
	if loopsToListener(target, listener) {
		return errors.New("gateway backend origin must not point to its listener")
	}
	return nil
}

func originPort(target *url.URL) string {
	if port := target.Port(); port != "" {
		return port
	}
	if target.Scheme == "https" {
		return "443"
	}
	return "80"
}

func canonicalHost(host string) string {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "localhost" {
		return "loopback"
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() {
			return "loopback"
		}
		return ip.String()
	}
	return host
}

func loopsToListener(target *url.URL, listener string) bool {
	host, port, err := net.SplitHostPort(listener)
	if err != nil || port == "0" || originPort(target) != port {
		return false
	}
	if canonicalHost(host) == canonicalHost(target.Hostname()) {
		return true
	}
	// Wildcard listeners also accept their machine's loopback addresses.
	ip := net.ParseIP(host)
	return (host == "" || (ip != nil && ip.IsUnspecified())) && canonicalHost(target.Hostname()) == "loopback"
}
