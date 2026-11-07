package media_test

import (
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
)

func TestPublicVideoLinkDownloadsActualTLSVideoAndRejectsPrivateRedirect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.mp4")
	if out, err := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "testsrc=size=160x90:rate=10", "-t", "1", "-pix_fmt", "yuv420p", "-y", path).CombinedOutput(); err != nil {
		t.Fatalf("video fixture: %s: %v", out, err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var origin string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/share":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<meta property="og:video" content="` + origin + `/clip">`))
		case "/twitter":
			_, _ = w.Write([]byte(`<meta name="twitter:player:stream" content="` + origin + `/clip">`))
		case "/playaddr":
			_, _ = w.Write([]byte(`<script>window.DATA={"video":{"play_addr":{"url_list":["` + origin + `/clip"]}}};</script>`))
		case "/guarded":
			_, _ = w.Write([]byte(`<html>登录后查看或完成验证码<a href="` + origin + `/clip">not verified metadata</a></html>`))
		case "/privatecdn":
			_, _ = w.Write([]byte(`<meta property="og:video" content="https://10.1.2.3/private">`))
		case "/redirectcdn":
			_, _ = w.Write([]byte(`<meta property="og:video" content="` + origin + `/external">`))
		case "/external":
			http.Redirect(w, r, "https://example.com/unapproved-origin", http.StatusFound)
		case "/clip":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write(body)
		case "/private":
			http.Redirect(w, r, "https://10.1.2.3/private", http.StatusFound)
		case "/large":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(strings.Repeat("x", (2<<20)+1)))
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	origin = server.URL
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	resolver, err := mediaflow.NewPublicVideoLinkResolver(mediaflow.VideoLinkResolverOptions{SourceOrigins: []string{origin}, TLSRootCAs: pool, AllowTestLoopbackTLS: true})
	if err != nil {
		t.Fatal(err)
	}
	file, err := resolver.Resolve(t.Context(), origin+"/share", 500<<20)
	if err != nil {
		t.Fatal(err)
	}
	if file.MIMEType != "video/mp4" || file.Size != int64(len(body)) {
		t.Fatal(file)
	}
	name := file.File.Name()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Fatal("temp video remains", err)
	}
	if _, err := resolver.Resolve(t.Context(), origin+"/private", 500<<20); err == nil {
		t.Fatal("private redirect accepted")
	}
	if _, err := resolver.Resolve(t.Context(), origin+"/large", 500<<20); err == nil {
		t.Fatal("unbounded source page accepted")
	}
	for _, path := range []string{"/twitter", "/playaddr"} {
		file, err := resolver.Resolve(t.Context(), origin+path, 500<<20)
		if err != nil {
			t.Fatal(path, err)
		}
		if file.Size != int64(len(body)) {
			t.Fatal("metadata did not download video", path)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"/guarded", "/privatecdn", "/redirectcdn"} {
		if _, err := resolver.Resolve(t.Context(), origin+path, 500<<20); err == nil {
			t.Fatal("unsafe or unavailable source accepted", path)
		}
	}
	if _, err := resolver.Resolve(t.Context(), origin+"/share", 16); err == nil {
		t.Fatal("video byte bound bypassed")
	}
}

func TestPublicVideoSourceRejectsNonPlatformOrigins(t *testing.T) {
	resolver, err := mediaflow.NewPublicVideoLinkResolver(mediaflow.VideoLinkResolverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"https://example.com/video", "https://douyin.com.evil.example/1", "http://v.douyin.com/1", "https://user:pass@v.douyin.com/1", "https://127.0.0.1/video"} {
		if _, err := resolver.Resolve(t.Context(), raw, 500<<20); err == nil {
			t.Fatal("unsafe source accepted", raw)
		}
	}
	if _, err := mediaflow.NewPublicVideoLinkResolver(mediaflow.VideoLinkResolverOptions{SourceOrigins: []string{"https://example.com"}}); !errors.Is(err, mediaflow.ErrUnsafeResultURL) {
		t.Fatal("production whitelist widened", err)
	}
}
