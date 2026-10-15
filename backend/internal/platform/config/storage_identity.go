package config

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

var cosBucketAPPID = regexp.MustCompile(`-[0-9]+$`)

// ValidateCOSOrigin binds the native SDK's BucketURL to the declared resource.
// Custom and accelerated domains need independent identity proof before use.
// Only explicit local fixtures may replace that origin with a loopback server.
func ValidateCOSOrigin(endpoint, bucket, region string, allowLoopback bool) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.User != nil || u.Hostname() == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return fmt.Errorf("%w: invalid COS origin", ErrInvalid)
	}
	if allowLoopback && loopbackOrigin(u) && (u.Scheme == "http" || u.Scheme == "https") {
		return nil
	}
	if u.Scheme != "https" || (u.Port() != "" && u.Port() != "443") || !cosBucketAPPID.MatchString(bucket) {
		return fmt.Errorf("%w: COS requires its matching native bucket origin", ErrInvalid)
	}
	host := strings.ToLower(u.Hostname())
	for _, suffix := range []string{".cos." + region + ".myqcloud.com", ".cos." + region + ".tencentcos.cn", ".cos-internal." + region + ".tencentcos.cn"} {
		if host == bucket+suffix {
			return nil
		}
	}
	return fmt.Errorf("%w: COS origin does not match declared bucket and region", ErrInvalid)
}

func loopbackOrigin(u *url.URL) bool {
	if strings.EqualFold(u.Hostname(), "localhost") {
		return true
	}
	ip := net.ParseIP(u.Hostname())
	return ip != nil && ip.IsLoopback()
}

// ResourceIdentity identifies the namespace actually addressed by a validated
// profile. Signing regions do not split one endpoint/bucket into two resources;
// COS's known native domain aliases all address the same bucket and region.
func (p StorageProfile) ResourceIdentity() string {
	if p.Provider == "cos" {
		u, err := url.Parse(p.Endpoint)
		if err == nil && !loopbackOrigin(u) {
			return strings.Join([]string{p.Provider, p.Region, p.Bucket}, "\x00")
		}
		return p.Provider + "\x00" + p.Endpoint
	}
	return strings.Join([]string{p.Provider, p.Endpoint, p.Bucket}, "\x00")
}
