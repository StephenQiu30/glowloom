package media_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

var storageRevisionAllocator = struct {
	sync.Mutex
	next   uint64
	bySeed map[string]string
}{next: uint64(time.Now().UnixNano()), bySeed: make(map[string]string)}

func storageEnvironmentRevision(db *gorm.DB, seed string) string {
	if _, err := config.ParseStorageRevision(seed); err == nil {
		return seed
	}
	storageRevisionAllocator.Lock()
	defer storageRevisionAllocator.Unlock()
	if revision, ok := storageRevisionAllocator.bySeed[seed]; ok {
		return revision
	}
	var current struct{ Revision string }
	read := db.Table("media.storage_route").Select("revision").Where("singleton = ?", 1).Take(&current)
	if read.Error == nil {
		if revision, err := config.ParseStorageRevision(current.Revision); err == nil && revision > storageRevisionAllocator.next {
			storageRevisionAllocator.next = revision
		}
	}
	var knownRevisions []string
	if err := db.Table("media.storage_config_revision").Pluck("revision", &knownRevisions).Error; err == nil {
		for _, known := range knownRevisions {
			if revision, err := config.ParseStorageRevision(known); err == nil && revision > storageRevisionAllocator.next {
				storageRevisionAllocator.next = revision
			}
		}
	}
	storageRevisionAllocator.next++
	revision := strconv.FormatUint(storageRevisionAllocator.next, 10)
	storageRevisionAllocator.bySeed[seed] = revision
	return revision
}

// storageEnvironmentFixture declares every fixture resource as writable, with a
// complete policy. Individual gate tests narrow modes explicitly before applying.
func storageEnvironmentFixture(t *testing.T, db *gorm.DB, revision, target string) mediaapp.StorageEnvironment {
	t.Helper()
	revision = storageEnvironmentRevision(db, revision)
	var profiles []mediaapp.StorageProfile
	if err := db.Table("media.storage_profile").Select("id, provider, endpoint, signing_endpoint, region, bucket, addressing_style").Order("id").Scan(&profiles).Error; err != nil {
		t.Fatal(err)
	}
	input := mediaapp.StorageEnvironment{Revision: revision, DefaultWriteProfile: target, UploadPolicy: uploadRoutingPolicy(), UploadSessionTTL: 24 * time.Hour}
	for _, profile := range profiles {
		input.Profiles = append(input.Profiles, mediaapp.StorageDeploymentProfile{StorageProfile: profile, Mode: mediaapp.StorageProfileModeReadWrite})
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	input.ConfigSHA256 = hex.EncodeToString(sum[:])
	return input
}
