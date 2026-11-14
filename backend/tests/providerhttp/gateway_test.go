package providerhttp_test

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/dns/dnsmessage"

	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/providerhttp"
)

type gatewayTransport func(*http.Request) (*http.Response, error)

func (f gatewayTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func gatewayConfig() catalog.ProviderConfig {
	spec, _ := catalog.GetAdapterSpec("openai")
	return catalog.ProviderConfig{AdapterKey: "openai", BaseURL: "https://gateway.example.com/v1", Region: spec.AllowedRegion, Capabilities: []string{"text"}, TimeoutSeconds: 5, ConcurrencyLimit: 1, RateLimitPerMin: 60}
}

// This package runs in its own test process. A loopback DNS server exercises the
// public constructor's real resolver without exporting a production test hook.
// These tests deliberately do not run in parallel while replacing the resolver.
type gatewayDNS struct {
	mu        sync.Mutex
	addresses []netip.Addr
	queries   atomic.Int32
}

func newGatewayDNS(t *testing.T, addresses []netip.Addr) *gatewayDNS {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	dns := &gatewayDNS{}
	dns.setAddresses(addresses)
	done := make(chan error, 1)
	go func() { done <- dns.serve(conn) }()
	previous := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "udp", conn.LocalAddr().String())
	}}
	t.Cleanup(func() {
		net.DefaultResolver = previous
		require.NoError(t, conn.Close())
		require.NoError(t, <-done)
	})
	return dns
}

func (d *gatewayDNS) setAddresses(addresses []netip.Addr) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.addresses = append([]netip.Addr(nil), addresses...)
}

func (d *gatewayDNS) serve(conn *net.UDPConn) error {
	buffer := make([]byte, 65535)
	for {
		n, peer, err := conn.ReadFromUDP(buffer)
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		if err != nil {
			return err
		}
		var question dnsmessage.Message
		if err := question.Unpack(buffer[:n]); err != nil {
			return err
		}
		response := dnsmessage.Message{Header: dnsmessage.Header{
			ID: question.ID, Response: true, Authoritative: true,
			RecursionDesired: question.RecursionDesired, RecursionAvailable: true,
		}, Questions: question.Questions}
		for _, query := range question.Questions {
			if !strings.EqualFold(query.Name.String(), "gateway.example.com.") {
				response.RCode = dnsmessage.RCodeNameError
				continue
			}
			d.queries.Add(1)
			d.mu.Lock()
			addresses := append([]netip.Addr(nil), d.addresses...)
			d.mu.Unlock()
			for _, address := range addresses {
				var body dnsmessage.ResourceBody
				switch {
				case query.Type == dnsmessage.TypeA && address.Is4():
					body = &dnsmessage.AResource{A: address.As4()}
				case query.Type == dnsmessage.TypeAAAA && address.Is6():
					body = &dnsmessage.AAAAResource{AAAA: address.As16()}
				default:
					continue
				}
				response.Answers = append(response.Answers, dnsmessage.Resource{
					Header: dnsmessage.ResourceHeader{Name: query.Name, Type: query.Type, Class: dnsmessage.ClassINET}, Body: body,
				})
			}
		}
		payload, err := response.Pack()
		if err != nil {
			return err
		}
		if _, err := conn.WriteToUDP(payload, peer); err != nil {
			return err
		}
	}
}

func TestGatewayRejectsPrivateDNSBeforeSendingCredentials(t *testing.T) {
	for _, addresses := range [][]netip.Addr{
		{netip.MustParseAddr("127.0.0.1")},
		{netip.MustParseAddr("169.254.169.254")},
		{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("10.0.0.1")},
		{netip.MustParseAddr("::ffff:192.168.1.1")},
	} {
		t.Run(addresses[len(addresses)-1].String(), func(t *testing.T) {
			dns := newGatewayDNS(t, addresses)
			client := providerhttp.NewClient(&http.Client{Transport: gatewayTransport(func(*http.Request) (*http.Response, error) {
				t.Fatal("unsafe destination reached transport")
				return nil, nil
			})})
			config := gatewayConfig()
			_, err := client.ReadModelIDs(t.Context(), config, "secret")
			require.ErrorIs(t, err, catalogapp.ErrModelDirectoryUnavailable)
			require.Equal(t, "unreachable", client.ProbeReachability(t.Context(), config).Result)
			require.Equal(t, "unreachable", client.Probe(t.Context(), config, "secret").Auth)
			require.Positive(t, dns.queries.Load())
		})
	}
}

func TestGatewayPinsResolvedIPRetainsHostAndNeverFollowsRedirect(t *testing.T) {
	dns := newGatewayDNS(t, []netip.Addr{netip.MustParseAddr("8.8.8.8")})
	requests := 0
	client := providerhttp.NewClient(&http.Client{Transport: gatewayTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		require.Equal(t, "8.8.8.8:8443", r.URL.Host)
		require.Equal(t, "gateway.example.com:8443", r.Host)
		require.Equal(t, "/api/v1/models", r.URL.Path)
		require.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
		dns.setAddresses([]netip.Addr{netip.MustParseAddr("127.0.0.1")})
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://127.0.0.1/secret"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})})
	config := gatewayConfig()
	config.BaseURL = "https://gateway.example.com:8443/api/v1/"
	_, err := client.ReadModelIDs(t.Context(), config, "fixture-key")
	require.ErrorIs(t, err, catalogapp.ErrModelDirectoryUnavailable)
	require.Equal(t, 1, requests)
	firstLookupQueries := dns.queries.Load()
	require.Positive(t, firstLookupQueries)
	_, err = client.ReadModelIDs(t.Context(), config, "fixture-key")
	require.ErrorIs(t, err, catalogapp.ErrModelDirectoryUnavailable)
	require.Equal(t, 1, requests)
	require.Greater(t, dns.queries.Load(), firstLookupQueries)
}

func TestGatewayKeepsProxyAndPinsActualDial(t *testing.T) {
	for _, useProxy := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "proxy"}[useProxy], func(t *testing.T) {
			newGatewayDNS(t, []netip.Addr{netip.MustParseAddr("8.8.8.8")})
			calls := 0
			transport := &http.Transport{
				Proxy: func(r *http.Request) (*url.URL, error) {
					require.Equal(t, "gateway.example.com", r.URL.Hostname())
					if useProxy {
						return url.Parse("http://proxy.internal:3128")
					}
					return nil, nil
				},
				DialContext: func(_ context.Context, _, address string) (net.Conn, error) {
					calls++
					want := "8.8.8.8:443"
					if useProxy {
						want = "proxy.internal:3128"
					}
					if address != want {
						return nil, fmt.Errorf("dial destination = %q, want %q", address, want)
					}
					return nil, errors.New("fixture stops before connection")
				},
			}
			client := providerhttp.NewClient(&http.Client{Transport: transport})
			_, err := client.ReadModelIDs(t.Context(), gatewayConfig(), "fixture-key")
			require.ErrorIs(t, err, catalogapp.ErrModelDirectoryUnavailable)
			require.Equal(t, 1, calls)
			require.Nil(t, transport.TLSClientConfig, "caller transport mutated")
		})
	}
}

func TestGatewayProxyCONNECTPinsTargetAndVerifiesOriginalTLSName(t *testing.T) {
	// Use the Go test certificate's example.com identity, with verification on.
	certificateServer := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(certificateServer.Close)
	certificates := certificateServer.TLS.Certificates
	roots := x509.NewCertPool()
	roots.AddCert(certificateServer.Certificate())
	for _, scheme := range []string{"http", "https"} {
		t.Run(scheme, func(t *testing.T) {
			dns := newGatewayDNS(t, []netip.Addr{netip.MustParseAddr("8.8.8.8")})
			proxyURL, err := url.Parse(scheme + "://proxy.example.com:3128")
			require.NoError(t, err)
			done := make(chan error, 1)
			transport := &http.Transport{
				Proxy: func(r *http.Request) (*url.URL, error) {
					require.Equal(t, "gateway.example.com", r.URL.Hostname())
					return proxyURL, nil
				},
				TLSClientConfig:     &tls.Config{RootCAs: roots},
				TLSHandshakeTimeout: time.Second,
				DialContext: func(_ context.Context, _, address string) (net.Conn, error) {
					if address != "proxy.example.com:3128" {
						return nil, fmt.Errorf("proxy dial destination = %q", address)
					}
					clientConn, proxyConn := net.Pipe()
					go func() { done <- serveGatewayTunnel(t.Context(), proxyConn, scheme == "https", certificates) }()
					return clientConn, nil
				},
			}
			client := providerhttp.NewClient(&http.Client{Transport: transport})
			models, err := client.ReadModelIDs(t.Context(), gatewayConfig(), "fixture-key")
			require.NoError(t, err)
			require.Equal(t, []string{"gateway-model"}, models)
			require.Positive(t, dns.queries.Load())
			require.NoError(t, <-done)
			require.Empty(t, transport.TLSClientConfig.ServerName)
		})
	}
}

func serveGatewayTunnel(ctx context.Context, conn net.Conn, secureProxy bool, certificates []tls.Certificate) error {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if secureProxy {
		secured := tls.Server(conn, &tls.Config{Certificates: certificates})
		if err := secured.HandshakeContext(ctx); err != nil {
			return err
		}
		if secured.ConnectionState().ServerName != "proxy.example.com" {
			return errors.New("HTTPS proxy received wrong SNI")
		}
		conn = secured
	}
	connect, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		return err
	}
	if connect.Method != http.MethodConnect || connect.Host != "8.8.8.8:443" || connect.Header.Get("Authorization") != "" {
		return errors.New("proxy received unpinned CONNECT or provider credential")
	}
	if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return err
	}
	secured := tls.Server(conn, &tls.Config{Certificates: certificates})
	if err := secured.HandshakeContext(ctx); err != nil {
		return err
	}
	if secured.ConnectionState().ServerName != "gateway.example.com" {
		return errors.New("gateway received wrong SNI")
	}
	request, err := http.ReadRequest(bufio.NewReader(secured))
	if err != nil {
		return err
	}
	if request.Host != "gateway.example.com" || request.URL.Path != "/v1/models" || request.Header.Get("Authorization") != "Bearer fixture-key" {
		return errors.New("gateway request lost original Host, path or credential")
	}
	body := `{"data":[{"id":"gateway-model"}]}`
	_, err = fmt.Fprintf(secured, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
	return err
}
