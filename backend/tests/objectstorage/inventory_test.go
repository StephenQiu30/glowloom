package objectstorage_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

func inventoryClient(t *testing.T, body string) (*objectstorage.Client, *atomic.Int32) {
	t.Helper()
	calls := new(atomic.Int32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Query().Get("prefix") != "projects/own/image/" || r.URL.Query().Get("list-type") != "2" || r.URL.Query().Get("delimiter") != "" {
			t.Errorf("unexpected inventory request")
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		if _, err := fmt.Fprint(w, body); err != nil {
			t.Error("write synthetic inventory response failed")
		}
	}))
	t.Cleanup(server.Close)
	client, err := objectstorage.Open(server.URL, "lanverse-test", "synthetic-access", "synthetic-secret", "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	return client, calls
}
func TestInventoryReadsExactPrefixAndRejectsIncompleteBound(t *testing.T) {
	body := `<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>lanverse-test</Name><Prefix>projects/own/image/</Prefix><IsTruncated>false</IsTruncated><Contents><Key>projects/own/image/a.png</Key><Size>11</Size></Contents><Contents><Key>projects/own/image/b.png</Key><Size>27</Size></Contents></ListBucketResult>`
	client, calls := inventoryClient(t, body)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	items, err := client.ListMetadata(ctx, "projects/own/image/", 2)
	if err != nil || len(items) != 2 || items[0].Key != "projects/own/image/a.png" || items[0].Size != 11 || items[1].Size != 27 {
		t.Fatalf("exact inventory shape: %v %v", items, err)
	}
	items, err = client.ListMetadata(ctx, "projects/own/image/", 1)
	if !errors.Is(err, objectstorage.ErrInventoryLimit) || items != nil {
		t.Fatalf("incomplete inventory returned as result: %v %v", items, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("unexpected extra requests")
	}
}
func TestInventoryRejectsForeignKeysDuplicateAndInvalidSize(t *testing.T) {
	for _, content := range []string{`<Contents><Key>projects/other/image/a.png</Key><Size>11</Size></Contents>`, `<Contents><Key>projects/own/image/a.png</Key><Size>-1</Size></Contents>`, `<Contents><Key>projects/own/image/a.png</Key><Size>11</Size></Contents><Contents><Key>projects/own/image/a.png</Key><Size>11</Size></Contents>`} {
		t.Run(content, func(t *testing.T) {
			body := `<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>lanverse-test</Name><IsTruncated>false</IsTruncated>` + content + `</ListBucketResult>`
			client, _ := inventoryClient(t, body)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			items, err := client.ListMetadata(ctx, "projects/own/image/", 10)
			if !errors.Is(err, objectstorage.ErrInvalidObject) || items != nil {
				t.Fatalf("untrusted inventory accepted: %v %v", items, err)
			}
		})
	}
}
func TestInventoryRejectsUnboundedPrefixAndCancelledContext(t *testing.T) {
	client, calls := inventoryClient(t, `<ListBucketResult/>`)
	for _, prefix := range []string{"", "/", "projects/", "projects/own", "projects/../image/", "projects//image/", "projects/own/\\image/", "projects/own/image/\x00"} {
		if items, err := client.ListMetadata(t.Context(), prefix, 10); !errors.Is(err, objectstorage.ErrInvalidObject) || items != nil {
			t.Fatalf("unsafe prefix accepted: %q %v", prefix, err)
		}
	}
	for _, limit := range []int{0, -1, 50001} {
		if _, err := client.ListMetadata(t.Context(), "projects/own/image/", limit); !errors.Is(err, objectstorage.ErrInvalidObject) {
			t.Fatalf("unbounded limit accepted")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if items, err := client.ListMetadata(ctx, "projects/own/image/", 10); !errors.Is(err, context.Canceled) || items != nil {
		t.Fatalf("cancelled inventory accepted: %v %v", items, err)
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid reads reached storage")
	}
}
