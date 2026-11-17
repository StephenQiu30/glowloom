package gateway_test

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDomainOperationTextDownloadPreservesOwnerAndAuthorization(t *testing.T) {
	const path = "/api/operations/11111111-1111-4111-8111-111111111111/text"
	const project = "22222222-2222-4222-8222-222222222222"
	text := strings.Repeat("original text", 20000)
	var calls atomic.Int32
	server, _ := domainProxy(t, func(owner string, w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, "execution", owner)
		assert.Equal(t, path, r.URL.Path)
		assert.Empty(t, r.Header.Get("X-Actor-ID"))
		if r.Header.Get("Cookie") != "session=synthetic" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("project_id") != project || r.Header.Get("X-Creation-Scope") != "synthetic-scope" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=generated-text.txt")
		_, _ = io.WriteString(w, text)
	})
	for _, step := range []struct {
		method, project, cookie string
		status                  int
	}{
		{"GET", project, "session=synthetic", 200},
		{"GET", "foreign-project", "session=synthetic", 403},
		{"GET", project, "", 401},
		{"POST", project, "session=synthetic", 404},
	} {
		req, err := http.NewRequestWithContext(t.Context(), step.method, server.URL+path+"?project_id="+step.project, nil)
		require.NoError(t, err)
		req.Header.Set("Cookie", step.cookie)
		req.Header.Set("X-Creation-Scope", "synthetic-scope")
		req.Header.Set("X-Actor-ID", "forged")
		res, err := server.Client().Do(req)
		require.NoError(t, err)
		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())
		require.Equal(t, step.status, res.StatusCode)
		if step.status == 200 {
			require.Equal(t, text, string(body))
			require.Equal(t, "text/plain; charset=utf-8", res.Header.Get("Content-Type"))
			require.Contains(t, res.Header.Get("Content-Disposition"), "generated-text.txt")
			require.Equal(t, "no-store", res.Header.Get("Cache-Control"))
		}
	}
	require.EqualValues(t, 3, calls.Load())
}
