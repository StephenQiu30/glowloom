package domain

import (
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ValidCreationResultURL checks an unsigned result from an authenticated supplier
// response. Music has documented CDN hosts. Visual has no documented fixed host;
// the downloader pins public DNS and permits only this exact HTTPS origin.
func ValidCreationResultURL(adapter, raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" || len(raw) > 16384 {
		return false
	}
	host := u.Hostname()
	if adapter == "volcengine_music" {
		if !strings.HasSuffix(host, "-default.douyinvod.com") || !strings.HasPrefix(host, "v") {
			return false
		}
		n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(host, "v"), "-default.douyinvod.com"))
		return err == nil && n >= 1 && n <= 50
	}
	if adapter == "volcengine_avatar" || adapter == "volcengine_motion" {
		return strings.Contains(host, ".") && net.ParseIP(host) == nil && !strings.HasSuffix(host, ".local") && !strings.HasSuffix(host, ".internal") && !strings.HasSuffix(host, ".localhost") && !strings.HasSuffix(host, ".")
	}
	return false
}
