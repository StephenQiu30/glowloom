package toolkit

import (
	"net"
	"net/http"
	"strings"
)

const BearerAuthScheme = "Bearer"

func ClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	remote := strings.TrimSpace(r.RemoteAddr)
	if host, _, err := net.SplitHostPort(remote); err == nil {
		return host
	}
	return remote
}

func BearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], BearerAuthScheme) || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}
