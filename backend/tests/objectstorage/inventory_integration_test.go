package objectstorage_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

func TestActualPrivateObjectInventory(t *testing.T) {
	path := os.Getenv("LV_TEST_OBJECT_INVENTORY_STORAGE_CONFIG")
	if path == "" {
		t.Skip("set isolated private inventory storage config")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm()&0077 != 0 {
		t.Fatal("private test configuration unavailable")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read isolated configuration failed")
	}
	var cfg struct {
		Endpoint  string `json:"endpoint"`
		Bucket    string `json:"bucket"`
		AccessKey string `json:"access_key"`
		SecretKey string `json:"secret_key"`
	}
	if json.Unmarshal(raw, &cfg) != nil || cfg.Endpoint != "http://127.0.0.1:19000" || cfg.Bucket != "lanverse-receipt-test" {
		t.Fatal("isolated inventory service required")
	}
	client, err := objectstorage.Open(cfg.Endpoint, cfg.Bucket, cfg.AccessKey, cfg.SecretKey, "")
	if err != nil {
		t.Fatal("isolated object client unavailable")
	}
	prefix := "tests/inventory/" + uuid.NewString() + "/"
	keys := []string{prefix + "original.txt", prefix + "unregistered.txt", "tests/inventory-other/" + uuid.NewString() + "/outside.txt"}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	t.Cleanup(func() {
		clean, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		for _, key := range keys {
			if client.Remove(clean, key) != nil {
				t.Error("cleanup exact synthetic object failed")
			}
		}
	})
	for i, key := range keys {
		data := strings.Repeat("x", 11+i)
		h := sha256.Sum256([]byte(data))
		if client.PutIfAbsent(ctx, key, strings.NewReader(data), int64(len(data)), "text/plain", hex.EncodeToString(h[:])) != nil {
			t.Fatal("write exact synthetic object failed")
		}
	}
	items, err := client.ListMetadata(ctx, prefix, 2)
	if err != nil || len(items) != 2 {
		t.Fatal("actual inventory incomplete")
	}
	var total int64
	for _, item := range items {
		stat, err := client.Stat(ctx, item.Key)
		if err != nil || stat.Size != item.Size || !strings.HasPrefix(item.Key, prefix) {
			t.Fatal("actual inventory metadata differs")
		}
		total += item.Size
	}
	if total != 23 {
		t.Fatal("actual unregistered bytes missing")
	}
	if items, err = client.ListMetadata(ctx, prefix, 1); !errors.Is(err, objectstorage.ErrInventoryLimit) || items != nil {
		t.Fatal("partial actual inventory accepted")
	}
	if client.Remove(ctx, keys[0]) != nil {
		t.Fatal("remove exact synthetic original failed")
	}
	items, err = client.ListMetadata(ctx, prefix, 2)
	if err != nil || len(items) != 1 || items[0].Key != keys[1] || items[0].Size != 12 {
		t.Fatal("actual removal absent from inventory")
	}
}
