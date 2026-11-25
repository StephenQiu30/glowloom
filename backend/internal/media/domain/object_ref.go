package domain

import (
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var (
	// ErrInvalidObjectRef rejects incomplete or unsafe physical identity.
	ErrInvalidObjectRef = errors.New("invalid physical object reference")
	objectProfileID     = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	objectBucket        = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
)

// ObjectLocationState is the persisted lifecycle of one physical object.
type ObjectLocationState string

// Object location states describe durable physical write, acceptance and removal facts.
const (
	ObjectLocationReserved      ObjectLocationState = "reserved"
	ObjectLocationPresent       ObjectLocationState = "present"
	ObjectLocationAccepted      ObjectLocationState = "accepted"
	ObjectLocationDeletePending ObjectLocationState = "delete_pending"
	ObjectLocationDeleted       ObjectLocationState = "deleted"
	ObjectLocationUnknown       ObjectLocationState = "unknown"
)

// ObjectRef is immutable storage identity. It contains no credential or access URL.
type ObjectRef struct {
	ProfileID string `json:"profile_id"`
	Bucket    string `json:"bucket"`
	Key       string `json:"key"`
}

// Validate checks identity syntax; resource ownership is enforced by its repository.
func (r ObjectRef) Validate() error {
	if !objectProfileID.MatchString(r.ProfileID) || !objectBucket.MatchString(r.Bucket) || strings.Contains(r.Bucket, "..") || r.Key == "" || len(r.Key) > 696 || strings.ContainsAny(r.Key, "\\\x00\r\n") {
		return ErrInvalidObjectRef
	}
	for _, segment := range strings.Split(r.Key, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return ErrInvalidObjectRef
		}
	}
	return nil
}

// ObjectLocation gives a physical object a stable database identity. A reserved
// row records intent before network I/O; it does not prove that bytes exist.
type ObjectLocation struct {
	ID       uuid.UUID
	Ref      ObjectRef
	State    ObjectLocationState
	ByteSize *int64
	SHA256   *string
	MIMEType *string
}

// Readable reports whether this stored location can serve the asset's exact key.
func (l *ObjectLocation) Readable(key string) bool {
	return l != nil && l.ID != uuid.Nil && l.Ref.Validate() == nil && l.Ref.Key == key && (l.State == ObjectLocationAccepted || l.State == ObjectLocationPresent)
}
