package media_test

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
)

type fixedResolver struct{ addresses []net.IPAddr }

func (r fixedResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return r.addresses, nil
}

func TestSecureDownloadAcceptsAllowedTLSImageAndCleansTempFile(t *testing.T) {
	var body bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&body, img); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body.Bytes())
	}))
	t.Cleanup(server.Close)
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	download, err := mediaflow.NewSecureDownloader(mediaflow.DownloadPolicy{
		AllowedOrigins: []string{server.URL}, AllowTestLoopbackTLS: true, TLSRootCAs: pool,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := download.Download(t.Context(), server.URL+"/image.png?token=private")
	if err != nil {
		t.Fatal(err)
	}
	if result.MIMEType != "image/png" || result.Size != int64(body.Len()) || len(result.SHA256) != 64 {
		t.Fatalf("download metadata = %+v", result)
	}
	probe, err := (mediaflow.FFProber{}).Probe(t.Context(), result)
	if err != nil || probe.Kind != "image" || probe.Extension != "png" ||
		probe.Width == nil || *probe.Width != 2 || probe.Height == nil || *probe.Height != 3 {
		t.Fatalf("probe = %+v, %v", probe, err)
	}
	name := result.File.Name()
	if err := result.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(name); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary result remains: %v", err)
	}
}

func TestSecureDownloadRejectsUntrustedDestinationBeforeRequest(t *testing.T) {
	for _, tc := range []struct {
		name    string
		allowed []string
		url     string
	}{
		{"plain HTTP", []string{"https://media.vendor.test"}, "http://media.vendor.test/out.png"},
		{"unlisted host", []string{"https://media.vendor.test"}, "https://other.vendor.test/out.png"},
		{"embedded credentials", []string{"https://media.vendor.test"}, "https://user:pass@media.vendor.test/out.png"},
		{"loopback literal", []string{"https://127.0.0.1"}, "https://127.0.0.1/out.png"},
		{"private literal", []string{"https://10.1.2.3"}, "https://10.1.2.3/out.png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			download, err := mediaflow.NewSecureDownloader(mediaflow.DownloadPolicy{AllowedOrigins: tc.allowed})
			if err != nil {
				t.Fatalf("construct downloader: %v", err)
			}
			if _, err := download.Download(t.Context(), tc.url); !errors.Is(err, mediaflow.ErrUnsafeResultURL) {
				t.Fatalf("download error = %v, want unsafe URL", err)
			}
		})
	}
}

func TestSecureDownloadRejectsPrivateDNSResolution(t *testing.T) {
	download, err := mediaflow.NewSecureDownloader(mediaflow.DownloadPolicy{
		AllowedOrigins: []string{"https://media.vendor.test"},
		Resolver:       fixedResolver{addresses: []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := download.Download(t.Context(), "https://media.vendor.test/out.png"); !errors.Is(err, mediaflow.ErrUnsafeResultURL) {
		t.Fatalf("private DNS result accepted: %v", err)
	}
}

func TestEmptyResultAllowlistDisablesDownloadsWithoutBlockingWorkerStartup(t *testing.T) {
	download, err := mediaflow.NewSecureDownloader(mediaflow.DownloadPolicy{})
	if err != nil {
		t.Fatalf("construct disabled downloader: %v", err)
	}
	if _, err := download.Download(t.Context(), "https://media.vendor.test/result.png"); !errors.Is(err, mediaflow.ErrUnsafeResultURL) {
		t.Fatalf("disabled downloader accepted result: %v", err)
	}
}

func TestSecureDownloadChecksRedirectAndSize(t *testing.T) {
	const png = "\x89PNG\r\n\x1a\n" + "abcdefghijklmnop"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "https://127.0.0.2/private", http.StatusFound)
		case "/oversize":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte(png))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	download, err := mediaflow.NewSecureDownloader(mediaflow.DownloadPolicy{
		AllowedOrigins:       []string{server.URL},
		AllowTestLoopbackTLS: true,
		TLSRootCAs:           pool,
		MaxBytes:             12,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := download.Download(t.Context(), server.URL+"/redirect"); !errors.Is(err, mediaflow.ErrUnsafeResultURL) {
		t.Fatalf("unsafe redirect accepted: %v", err)
	}
	if _, err := download.Download(t.Context(), server.URL+"/oversize"); !errors.Is(err, mediaflow.ErrResultTooLarge) {
		t.Fatalf("oversize result accepted: %v", err)
	}
}

func TestSecureDownloadDoesNotForwardSignedURLAsRedirectReferer(t *testing.T) {
	var imageBody bytes.Buffer
	if err := png.Encode(&imageBody, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	referer := make(chan string, 1)
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		referer <- r.Header.Get("Referer")
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imageBody.Bytes())
	}))
	t.Cleanup(target.Close)
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/image.png", http.StatusFound)
	}))
	t.Cleanup(source.Close)
	pool := x509.NewCertPool()
	pool.AddCert(source.Certificate())
	pool.AddCert(target.Certificate())
	download, err := mediaflow.NewSecureDownloader(mediaflow.DownloadPolicy{
		AllowedOrigins: []string{source.URL, target.URL}, AllowTestLoopbackTLS: true, TLSRootCAs: pool,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := download.Download(t.Context(), source.URL+"/redirect?token=secret")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = result.Close() })
	if got := <-referer; got != "" {
		t.Fatalf("signed URL leaked in redirect Referer: %q", got)
	}
}

func TestSecureDownloadDistinguishesExpiredResultFromTransientFailure(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		want   error
	}{
		{name: "forbidden signed URL", status: http.StatusForbidden, want: mediaflow.ErrResultExpired},
		{name: "gone signed URL", status: http.StatusGone, want: mediaflow.ErrResultExpired},
		{name: "temporary provider failure", status: http.StatusServiceUnavailable, want: mediaflow.ErrResultUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
			}))
			t.Cleanup(server.Close)
			pool := x509.NewCertPool()
			pool.AddCert(server.Certificate())
			download, err := mediaflow.NewSecureDownloader(mediaflow.DownloadPolicy{
				AllowedOrigins: []string{server.URL}, AllowTestLoopbackTLS: true, TLSRootCAs: pool,
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := download.Download(t.Context(), server.URL+"/result?token=secret"); !errors.Is(err, test.want) {
				t.Fatalf("status %d: got %v, want %v", test.status, err, test.want)
			}
		})
	}
}
