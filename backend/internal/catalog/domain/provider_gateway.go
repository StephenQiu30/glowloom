package domain

import (
	"net/netip"
	"net/url"
	"path"
	"strconv"
	"strings"
)

// IsCustomGateway distinguishes third-party model evidence and pricing from the
// installed OpenAI endpoint. It does not assert that the config is valid.
func (c ProviderConfig) IsCustomGateway() bool {
	spec, ok := GetAdapterSpec(c.AdapterKey)
	return ok && c.AdapterKey == "openai" && c.BaseURL != spec.AllowedBaseURL
}

// ValidateGatewayBaseURL validates the persisted address without performing DNS.
// The HTTP boundary must also resolve and pin a public address for every request.
func ValidateGatewayBaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || u.Scheme != "https" || u.Host == "" || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") || u.RawPath != "" {
		return ErrInvalidPreset
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return ErrInvalidPreset
		}
	} else if strings.HasSuffix(u.Host, ":") {
		return ErrInvalidPreset
	}
	basePath := strings.TrimSuffix(u.Path, "/")
	if basePath != "" && path.Clean(basePath) != basePath || strings.ContainsAny(u.Path, "\\\r\n\x00") {
		return ErrInvalidPreset
	}
	host := strings.ToLower(u.Hostname())
	if ip, err := netip.ParseAddr(host); err == nil {
		if !IsPublicProviderIP(ip) {
			return ErrInvalidPreset
		}
		return nil
	}
	if len(host) > 253 || !strings.Contains(host, ".") {
		return ErrInvalidPreset
	}
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return ErrInvalidPreset
		}
		for _, ch := range label {
			if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '-' {
				return ErrInvalidPreset
			}
		}
	}
	last := labels[len(labels)-1]
	if strings.Trim(last, "0123456789") == "" {
		return ErrInvalidPreset
	}
	switch last {
	case "localhost", "local", "internal", "home", "lan", "invalid", "test", "onion":
		return ErrInvalidPreset
	}
	return nil
}

var nonPublicProviderPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("127.0.0.0/8"),
	// Azure's platform virtual address is publicly numbered but host-local.
	netip.MustParsePrefix("168.63.129.16/32"),
	netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
}

// IsPublicProviderIP excludes special-purpose addresses, including IPv4-mapped
// IPv6 and transition ranges that could route back to private networks.
func IsPublicProviderIP(ip netip.Addr) bool {
	if !ip.IsValid() || ip.Zone() != "" {
		return false
	}
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return false
	}
	for _, prefix := range nonPublicProviderPrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}
