// Command cosfixture runs an isolated, in-memory COS protocol fixture for CI.
package main

import (
	"bytes"
	"encoding/xml"
	"hash/crc64"
	"io"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type object struct {
	body        []byte
	contentType string
	digest      string
}

type fixture struct {
	mu      sync.Mutex
	objects map[string]object
}

var crcTable = crc64.MakeTable(crc64.ECMA)

func main() {
	server := http.Server{
		Addr:              "127.0.0.1:19000",
		Handler:           &fixture{objects: make(map[string]object)},
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}

func (f *fixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "q-sign-algorithm=sha1&") && r.URL.Query().Get("q-signature") == "" {
		writeError(w, http.StatusForbidden, "AccessDenied")
		return
	}

	key := strings.TrimPrefix(r.URL.Path, "/")
	if key == "" {
		switch r.Method {
		case http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			f.list(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		return
	}

	switch r.Method {
	case http.MethodPut:
		f.put(w, r, key)
	case http.MethodHead, http.MethodGet:
		f.get(w, r, key)
	case http.MethodDelete:
		f.mu.Lock()
		delete(f.objects, key)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (f *fixture) put(w http.ResponseWriter, r *http.Request, key string) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 512<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "InvalidArgument")
		return
	}
	f.mu.Lock()
	_, exists := f.objects[key]
	if exists && r.Header.Get("X-Cos-Forbid-Overwrite") == "true" {
		f.mu.Unlock()
		writeError(w, http.StatusConflict, "FileAlreadyExists")
		return
	}
	f.objects[key] = object{body: data, contentType: r.Header.Get("Content-Type"), digest: r.Header.Get("X-Cos-Meta-Sha256")}
	f.mu.Unlock()
	w.Header().Set("X-Cos-Hash-Crc64ecma", strconv.FormatUint(crc64.Checksum(data, crcTable), 10))
	w.Header().Set("ETag", `"fixture"`)
	w.WriteHeader(http.StatusOK)
}

func (f *fixture) get(w http.ResponseWriter, r *http.Request, key string) {
	f.mu.Lock()
	item, ok := f.objects[key]
	f.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound, "NoSuchKey")
		return
	}
	w.Header().Set("Content-Type", item.contentType)
	w.Header().Set("X-Cos-Meta-Sha256", item.digest)
	w.Header().Set("ETag", `"fixture"`)
	http.ServeContent(w, r, key, time.Time{}, bytes.NewReader(item.body))
}

func (f *fixture) list(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	prefix, marker := query.Get("prefix"), query.Get("marker")
	limit, err := strconv.Atoi(query.Get("max-keys"))
	if err != nil || limit < 1 || limit > 1000 {
		limit = 1000
	}
	type entry struct {
		Key  string
		Size int
	}
	page := struct {
		XMLName     xml.Name `xml:"ListBucketResult"`
		IsTruncated bool
		NextMarker  string `xml:",omitempty"`
		Contents    []entry
	}{}
	f.mu.Lock()
	var keys []string
	for key := range f.objects {
		if strings.HasPrefix(key, prefix) && key > marker {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	page.IsTruncated = len(keys) > limit
	if page.IsTruncated {
		keys = keys[:limit]
		page.NextMarker = keys[len(keys)-1]
	}
	for _, key := range keys {
		page.Contents = append(page.Contents, entry{Key: key, Size: len(f.objects[key].body)})
	}
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/xml")
	_ = xml.NewEncoder(w).Encode(page)
}

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_ = xml.NewEncoder(w).Encode(struct {
		XMLName xml.Name `xml:"Error"`
		Code    string
	}{Code: code})
}
