// Package writefence defines the durable, writer-local final-commit barrier.
// Authorities are installed only by adapters that authenticate the corresponding
// issuer. These values are not a replacement for account or resource policy.
package writefence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// Issuer identifies a policy authority, never a caller-supplied service label.
type Issuer string

// Supported policy issuers are fixed by the service ownership contract.
const (
	Identity Issuer = "identity"
	Creation Issuer = "creation"
)

// Kind identifies an authority whose monotonic epoch the writer must lock.
type Kind string

// Supported authority kinds cannot be extended by an untrusted request.
const (
	Account    Kind = "account"
	Session    Kind = "session"
	Scope      Kind = "scope"
	Project    Kind = "project"
	AgentGrant Kind = "agent_grant"
)

// Errors distinguish invalid input, stale policy and durable command outcomes.
var (
	ErrInvalid     = errors.New("invalid write fence command")
	ErrFenced      = errors.New("write authority is absent, closed or stale")
	ErrConflict    = errors.New("write command identity conflicts with durable facts")
	ErrStale       = errors.New("write control epoch is stale")
	ErrNotFound    = errors.New("write command not found")
	ErrCommitted   = errors.New("write command already committed")
	ErrFrozen      = errors.New("write command permanently frozen")
	ErrTransaction = errors.New("write fence requires an existing transaction")
	ErrUnavailable = errors.New("write fence unavailable")
)

// Resource limits bound control facts and private recovery evidence.
const (
	MaxAuthorities  = 16
	MaxReceiptBytes = 1 << 20
)

// Authority binds a command to one exact policy generation.
type Authority struct {
	Issuer Issuer    `json:"issuer"`
	Kind   Kind      `json:"kind"`
	ID     uuid.UUID `json:"id"`
	Epoch  int64     `json:"epoch"`
}

// Validate rejects unknown issuer/kind combinations and missing generations.
func (a Authority) Validate() error {
	if a.ID == uuid.Nil || a.Epoch <= 0 {
		return ErrInvalid
	}
	switch a.Issuer {
	case Identity:
		if a.Kind == Account || a.Kind == Session {
			return nil
		}
	case Creation:
		if a.Kind == Scope || a.Kind == Project || a.Kind == AgentGrant {
			return nil
		}
	}
	return ErrInvalid
}

// Key defines the shared lock order without conflating policy generations.
func (a Authority) Key() string {
	return string(a.Issuer) + "/" + string(a.Kind) + "/" + a.ID.String()
}

// Control is an issuer-authenticated decision, retained under its original ID.
// Reopening an authority requires a strictly higher epoch, not an old open ACK.
type Control struct {
	Authority
	Blocked        bool
	CoordinationID uuid.UUID
}

// Validate requires a stable coordination identity for control replay.
func (c Control) Validate() error {
	if c.CoordinationID == uuid.Nil {
		return ErrInvalid
	}
	return c.Authority.Validate()
}

// Command is the immutable identity of an admitted business effect. SHA256 must
// be the existing business request digest; migrating services must not change it.
type Command struct {
	ID          uuid.UUID   `json:"id"`
	SHA256      string      `json:"sha256"`
	Action      string      `json:"action"`
	Authorities []Authority `json:"authorities"`
}

// AuthorityDigest canonicalizes only the authority set, not business identity.
// It never changes caller-owned ordering or accepts duplicate authority keys.
func (c Command) AuthorityDigest() (string, error) {
	if c.ID == uuid.Nil || len(c.SHA256) != 64 || c.SHA256 != strings.ToLower(c.SHA256) {
		return "", ErrInvalid
	}
	if _, err := hex.DecodeString(c.SHA256); err != nil {
		return "", ErrInvalid
	}
	if len(c.Action) == 0 || len(c.Action) > 128 || len(c.Authorities) == 0 || len(c.Authorities) > MaxAuthorities {
		return "", ErrInvalid
	}
	for _, b := range []byte(c.Action) {
		switch {
		case b >= 'a' && b <= 'z', b >= '0' && b <= '9', b == '.', b == '_', b == '-':
		default:
			return "", ErrInvalid
		}
	}
	authorities := append([]Authority(nil), c.Authorities...)
	sort.Slice(authorities, func(i, j int) bool { return authorities[i].Key() < authorities[j].Key() })
	var encoded strings.Builder
	for i, authority := range authorities {
		if err := authority.Validate(); err != nil {
			return "", err
		}
		if i > 0 && authorities[i-1].Key() == authority.Key() {
			return "", ErrInvalid
		}
		encoded.WriteString(authority.Key())
		encoded.WriteByte(':')
		encoded.WriteString(strconv.FormatInt(authority.Epoch, 10))
		encoded.WriteByte('\n')
	}
	digest := sha256.Sum256([]byte(encoded.String()))
	return hex.EncodeToString(digest[:]), nil
}

// State is durable effect state. No timeout or TTL implies a transition.
type State string

// Durable states retain unknown responsibility until committed or frozen.
const (
	Admitted  State = "admitted"
	Committed State = "committed"
	Frozen    State = "frozen"
)

// Record is private recovery evidence. Reading a record never grants new access.
type Record struct {
	Command Command
	State   State
	Receipt json.RawMessage
}

// CanonicalReceipt bounds a closed effect receipt and normalizes JSON whitespace
// and object ordering so a lost-ACK replay cannot overwrite a different outcome.
func CanonicalReceipt(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > MaxReceiptBytes {
		return nil, ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil || object == nil {
		return nil, ErrInvalid
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, ErrInvalid
	}
	remaining := MaxReceiptBytes
	if _, err := normalizeReceiptValue(object, &remaining, 0); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(object)
	if err != nil || len(canonical) > MaxReceiptBytes {
		return nil, ErrInvalid
	}
	return canonical, nil
}
