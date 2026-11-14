package providerhttp

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

var errGatewayAddress = errors.New("provider gateway must resolve only to public HTTPS destinations")

// do shares the same gateway fence across generation, credential checks,
// reachability and model-directory reads. Installed endpoints keep their original
// transport behavior, including explicitly injected fixture transports.
func (c *Client) do(config catalog.ProviderConfig, request *http.Request) (*http.Response, error) {
	if !config.IsCustomGateway() {
		return c.client.Do(request)
	}
	blocked := func(err error) (*http.Response, error) {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: err}
	}
	if catalog.ValidateGatewayBaseURL(config.BaseURL) != nil {
		return blocked(errGatewayAddress)
	}
	base, _ := url.Parse(config.BaseURL)
	if request.URL.Scheme != "https" || !strings.EqualFold(request.URL.Host, base.Host) {
		return blocked(errGatewayAddress)
	}
	host := request.URL.Hostname()
	var addresses []netip.Addr
	if address, err := netip.ParseAddr(host); err == nil {
		addresses = []netip.Addr{address}
	} else {
		var err error
		addresses, err = c.lookupIP(request.Context(), "ip", host)
		if err != nil {
			return blocked(err)
		}
	}
	if len(addresses) == 0 {
		return blocked(errGatewayAddress)
	}
	for _, address := range addresses {
		if !catalog.IsPublicProviderIP(address) {
			return blocked(errGatewayAddress)
		}
	}
	port := request.URL.Port()
	if port == "" {
		port = "443"
	}
	pinned := request.Clone(request.Context())
	pinned.URL.Host = net.JoinHostPort(addresses[0].Unmap().String(), port)
	pinned.Host = request.URL.Host
	client := c.client
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	if source, ok := transport.(*http.Transport); ok {
		var proxy *url.URL
		if source.Proxy != nil {
			var err error
			// Select with the original hostname so NO_PROXY and caller routing
			// rules retain their established meaning.
			proxy, err = source.Proxy(request)
			if err != nil {
				return blocked(err)
			}
		}
		bounded := source.Clone()
		bounded.Proxy = func(*http.Request) (*url.URL, error) { return proxy, nil }
		bounded.DisableKeepAlives = true
		if bounded.TLSClientConfig == nil {
			bounded.TLSClientConfig = &tls.Config{}
		}
		bounded.TLSClientConfig.ServerName = host
		// CONNECT uses the pinned IP; TLS still verifies and sends SNI for the
		// original hostname. HTTPS proxies need their own TLS server name.
		if proxy != nil && proxy.Scheme == "https" && source.DialTLSContext == nil {
			bounded.DialTLSContext = gatewayProxyTLS(source, proxy.Hostname())
		}
		client.Transport = bounded
		defer bounded.CloseIdleConnections()
	} else {
		// An injected RoundTripper remains in control of I/O but receives the
		// same pinned destination and original Host as a standard transport.
		client.Transport = transport
	}
	return client.Do(pinned)
}

func gatewayProxyTLS(source *http.Transport, serverName string) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		dial := source.DialContext
		if dial == nil {
			dial = (&net.Dialer{}).DialContext
		}
		conn, err := dial(ctx, network, address)
		if err != nil {
			return nil, err
		}
		config := &tls.Config{}
		if source.TLSClientConfig != nil {
			config = source.TLSClientConfig.Clone()
		}
		config.ServerName = serverName
		secured := tls.Client(conn, config)
		if source.TLSHandshakeTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, source.TLSHandshakeTimeout)
			defer cancel()
		}
		if err := secured.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, err
		}
		return secured, nil
	}
}
