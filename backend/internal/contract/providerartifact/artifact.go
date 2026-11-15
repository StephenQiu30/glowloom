// Package providerartifact defines opaque immutable provider-byte transfer facts.
// It contains no authorization, storage routing, pricing or media policy.
package providerartifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// ChunkSize is the maximum immutable source frame in bytes.
const ChunkSize = 256 << 10

var (
	// ErrInvalid means an original transfer envelope is malformed.
	ErrInvalid = errors.New("invalid provider artifact transfer")
	// ErrConflict means an immutable identity or receipt does not match.
	ErrConflict = errors.New("provider artifact transfer conflict")
	// ErrUnknown preserves unresolved responsibility after an uncertain transfer.
	ErrUnknown = errors.New("provider artifact transfer is unresolved")
)

// Request identifies one original artifact without a URL, key or credential.
type Request struct {
	Command               writefence.Command `json:"command"`
	DeliveryID            uuid.UUID          `json:"delivery_id"`
	ProviderCallID        uuid.UUID          `json:"provider_call_id"`
	ProviderCallCreatedAt time.Time          `json:"provider_call_created_at"`
	OperationID           uuid.UUID          `json:"operation_id"`
	ProjectID             uuid.UUID          `json:"project_id"`
	CallAction            string             `json:"call_action"`
	CallAttempt           int32              `json:"call_attempt"`
	RequestKey            string             `json:"request_key"`
	Sequence              int                `json:"sequence"`
	ByteSize              int64              `json:"byte_size"`
	SHA256                string             `json:"sha256"`
	ManifestSHA256        string             `json:"manifest_sha256"`
	MIME                  string             `json:"mime"`
	Processing            string             `json:"processing,omitempty"`
}

// Digest binds the command, complete authorities and immutable source envelope.
func (r Request) Digest() string {
	command := r.Command
	command.SHA256 = strings.Repeat("0", 64)
	authority, err := command.AuthorityDigest()
	if err != nil {
		return ""
	}
	r.ProviderCallCreatedAt = r.ProviderCallCreatedAt.UTC()
	raw, _ := json.Marshal(struct {
		Request   Request `json:"request"`
		Authority string  `json:"authority"`
	}{Request: func() Request { v := r; v.Command.SHA256 = ""; return v }(), Authority: authority})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Validate checks only bounded wire facts. Owners enforce their business policies.
func (r Request) Validate() error {
	if _, err := r.Command.AuthorityDigest(); err != nil {
		return ErrInvalid
	}
	if len(r.Processing) > 64 {
		return ErrInvalid
	}
	if r.Command.Action != "media.provider_artifact.stage" || r.Command.SHA256 != r.Digest() || r.DeliveryID == uuid.Nil || r.ProviderCallID == uuid.Nil || r.ProviderCallCreatedAt.IsZero() || r.OperationID == uuid.Nil || r.ProjectID == uuid.Nil || (r.CallAction != "submit" && r.CallAction != "query") || r.CallAttempt < 1 || len(r.RequestKey) < 1 || len(r.RequestKey) > 256 || r.Sequence < 0 || r.Sequence > 2147483647 || r.ByteSize < 1 || r.ByteSize > 512<<20 || !validHash(r.SHA256) || !validHash(r.ManifestSHA256) || len(r.MIME) < 1 || len(r.MIME) > 128 {
		return ErrInvalid
	}
	return nil
}

func validHash(v string) bool {
	if len(v) != 64 {
		return false
	}
	_, err := hex.DecodeString(v)
	if err != nil {
		return false
	}
	for _, c := range v {
		if c >= 'A' && c <= 'F' {
			return false
		}
	}
	return true
}

// Equal binds a replay to the exact original request, including first timestamps.
func (r Request) Equal(other Request) bool {
	return r.Validate() == nil && other.Validate() == nil && r.Digest() == other.Digest()
}

// Descriptor is the complete immutable canonical byte receipt, without storage identity.
type Descriptor struct {
	ByteSize   int64  `json:"byte_size"`
	SHA256     string `json:"sha256"`
	MIME       string `json:"mime"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

// Validate checks only bounded wire facts; each owner checks its processing policy.
func (d Descriptor) Validate() error {
	if d.ByteSize < 1 || d.ByteSize > 512<<20 || !validHash(d.SHA256) || len(d.MIME) < 1 || len(d.MIME) > 128 || d.DurationMS < 0 || d.DurationMS > 2147483647 {
		return ErrInvalid
	}
	return nil
}

// Receipt is Media's permanent byte responsibility. ArtifactID is opaque.
type Receipt struct {
	Request    Request     `json:"request"`
	ArtifactID uuid.UUID   `json:"artifact_id"`
	State      string      `json:"state"`
	Canonical  *Descriptor `json:"canonical,omitempty"`
}

// Matches rejects substituted or incomplete acknowledgements.
func (r Receipt) Matches(request Request) bool {
	return r.ArtifactID == request.DeliveryID && r.State == "present" && r.Request.Equal(request) && (r.Canonical == nil || r.Canonical.Validate() == nil)
}
