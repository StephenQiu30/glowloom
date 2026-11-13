// Package memory stores a bounded process-local diagnostic sample without infrastructure.
package memory

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/observability/domain"
)

type entry struct {
	record domain.DiagnosticRecord
	seq    uint64
}

// Buffer owns synchronized bounded records and a process-local cursor signing key.
type Buffer struct {
	mu           sync.Mutex
	records      []entry
	head, length int
	retention    time.Duration
	clock        func() time.Time
	started      time.Time
	seq, evicted uint64
	key          [32]byte
}

// NewBuffer bounds retention and capacity even for incorrectly configured callers.
func NewBuffer(capacity int, retention time.Duration, clock func() time.Time) *Buffer {
	if capacity < 1 || capacity > 10000 {
		capacity = 4096
	}
	if retention <= 0 || retention > 7*24*time.Hour {
		retention = 24 * time.Hour
	}
	if clock == nil {
		clock = time.Now
	}
	b := &Buffer{retention: retention, clock: clock, started: clock().UTC(), records: make([]entry, capacity)}
	// A UUID-derived key still isolates restarts if the OS RNG unexpectedly fails.
	if _, err := rand.Read(b.key[:]); err != nil {
		a, c := uuid.New(), uuid.New()
		copy(b.key[:16], a[:])
		copy(b.key[16:], c[:])
	}
	return b
}

// Append never waits for external I/O and cannot affect the business transaction.
func (b *Buffer) Append(r domain.DiagnosticRecord) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prune()
	r.ID = uuid.New()
	r.Service = "backend"
	r.Role = "api"
	r.Release = "unknown"
	r.Event = "http.request.completed"
	if r.Timestamp.IsZero() {
		r.Timestamp = b.clock().UTC()
	}
	r.Timestamp = r.Timestamp.UTC()
	r.Level = "info"
	r.ErrorClass = ""
	if r.Status >= 500 {
		r.Level = "error"
		r.ErrorClass = "server_error"
	} else if r.Status >= 400 {
		r.Level = "warn"
		r.ErrorClass = "client_error"
	}
	if r.Status == 429 {
		r.ErrorClass = "rate_limited"
	}
	if r.Panic {
		r.Level = "error"
		r.ErrorClass = "internal_error"
	}
	b.seq++
	if b.length == len(b.records) {
		b.removeOldest()
	}
	b.records[b.index(b.length)] = entry{r, b.seq}
	b.length++
}

func (b *Buffer) index(offset int) int { return (b.head + offset) % len(b.records) }

func (b *Buffer) removeOldest() {
	b.records[b.head] = entry{}
	b.head = b.index(1)
	b.length--
	b.evicted++
}

func (b *Buffer) prune() {
	cutoff := b.clock().Add(-b.retention)
	for b.length > 0 && b.records[b.head].record.Timestamp.Before(cutoff) {
		b.removeOldest()
	}
}
func (b *Buffer) coverage() domain.Coverage {
	c := domain.Coverage{Status: "partial", Source: "process_memory", StartedAt: b.started, RetentionSeconds: int64(b.retention / time.Second), Capacity: len(b.records), Retained: b.length, Evicted: b.evicted, RestartLoss: true, MissingServices: []string{"agent", "canvas", "frontend"}}
	if b.length > 0 {
		t := b.records[b.head].record.Timestamp
		c.OldestAt = &t
	}
	return c
}

type cursor struct {
	Actor                   uuid.UUID
	From, To, AsOf          time.Time
	Snapshot, Before, Floor uint64
	Fingerprint             string
}

func fingerprint(f domain.Filter) string {
	sum := sha256.Sum256([]byte(f.From.Format(time.RFC3339Nano) + "|" + f.To.Format(time.RFC3339Nano) + "|" + f.Level + "|" + f.Service + "|" + f.RequestID + "|" + f.TraceID + "|" + strconv.Itoa(f.Limit)))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
func (b *Buffer) encode(c cursor) string {
	data, _ := json.Marshal(c)
	mac := hmac.New(sha256.New, b.key[:])
	_, _ = mac.Write(data)
	return base64.RawURLEncoding.EncodeToString(append(data, mac.Sum(nil)...))
}
func (b *Buffer) position(f *domain.Filter, actor uuid.UUID) (cursor, error) {
	now := b.clock().UTC()
	p := cursor{Actor: actor, From: f.From, To: f.To, AsOf: now, Snapshot: b.seq, Before: b.seq + 1, Floor: b.seq + 1}
	if b.length > 0 {
		p.Floor = b.records[b.head].seq
	}
	if f.Cursor != "" {
		data, err := base64.RawURLEncoding.DecodeString(f.Cursor)
		if err != nil || len(data) < 33 || len(data) > 1536 {
			return p, domain.ErrInvalidFilter
		}
		body, signature := data[:len(data)-32], data[len(data)-32:]
		mac := hmac.New(sha256.New, b.key[:])
		_, _ = mac.Write(body)
		if !hmac.Equal(mac.Sum(nil), signature) || json.Unmarshal(body, &p) != nil {
			return p, domain.ErrInvalidFilter
		}
		if !f.FromSet {
			f.From = p.From
		}
		if !f.ToSet {
			f.To = p.To
		}
		if p.Actor != actor || p.Fingerprint != fingerprint(*f) || p.AsOf.After(now) || now.Sub(p.AsOf) >= 10*time.Minute || p.Snapshot > b.seq || p.Before > p.Snapshot+1 || p.Before == 0 {
			return p, domain.ErrInvalidFilter
		}
		if b.evicted >= p.Floor {
			return p, domain.ErrInvalidFilter
		}
		f.Cursor = "" // Validate the complete restored range before reading records.
	}
	if err := f.Validate(now); err != nil {
		return p, err
	}
	p.Fingerprint = fingerprint(*f)
	return p, nil
}
func matches(r domain.DiagnosticRecord, f domain.Filter) bool {
	return !r.Timestamp.Before(f.From) && r.Timestamp.Before(f.To) && (f.Level == "" || r.Level == f.Level) && (f.Service == "" || r.Service == f.Service) && (f.RequestID == "" || r.RequestID == f.RequestID) && (f.TraceID == "" || r.TraceID == f.TraceID)
}

// List signs the actor, filters, range and retained snapshot into each cursor.
func (b *Buffer) List(f domain.Filter, actor uuid.UUID) (domain.Page, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prune()
	p, err := b.position(&f, actor)
	if err != nil {
		return domain.Page{}, err
	}
	first := p
	first.Before = p.Snapshot + 1
	out := domain.Page{FirstCursor: b.encode(first), Items: []domain.DiagnosticRecord{}, AsOf: p.AsOf, From: f.From, To: f.To, Coverage: b.coverage()}
	for i := b.length - 1; i >= 0; i-- {
		e := b.records[b.index(i)]
		if e.seq > p.Snapshot || e.seq >= p.Before || !matches(e.record, f) {
			continue
		}
		if len(out.Items) == f.Limit {
			token := b.encode(p)
			out.NextCursor = &token
			break
		}
		out.Items = append(out.Items, e.record)
		p.Before = e.seq
	}
	return out, nil
}

// Detail finds an opaque record identity only within current retention.
func (b *Buffer) Detail(id uuid.UUID) (domain.Detail, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prune()
	for i := range b.length {
		e := b.records[b.index(i)]
		if e.record.ID == id {
			return domain.Detail{Record: e.record, Coverage: b.coverage()}, nil
		}
	}
	return domain.Detail{}, domain.ErrNotFound
}

// Overview computes quantiles over this single retained process population.
func (b *Buffer) Overview(f domain.Filter, actor uuid.UUID) (domain.Overview, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prune()
	p, err := b.position(&f, actor)
	if err != nil {
		return domain.Overview{}, err
	}
	out := domain.Overview{AsOf: p.AsOf, From: f.From, To: f.To, Coverage: b.coverage(), Unavailable: []string{"service_uptime", "cpu", "disk", "db_pool", "task_recovery", "cross_service", "long_retention", "release_identity"}}
	var samples []float64
	for i := range b.length {
		e := b.records[b.index(i)]
		r := e.record
		if e.seq > p.Snapshot || !matches(r, f) {
			continue
		}
		out.Requests++
		if r.Panic {
			out.Panics++
		}
		samples = append(samples, r.ElapsedMS)
		if r.Status >= 500 {
			out.ServerErrors++
		} else if r.Status >= 400 {
			out.ClientErrors++
		}
		if r.Status == 429 {
			out.RateLimited++
		}
	}
	if out.Requests > 0 {
		rate := float64(out.ServerErrors) / float64(out.Requests)
		out.ServerErrorRate = &rate
		sort.Float64s(samples)
		p50 := samples[int(math.Ceil(float64(len(samples))*0.50))-1]
		p95 := samples[int(math.Ceil(float64(len(samples))*0.95))-1]
		out.P50MS = &p50
		out.P95MS = &p95
	}
	return out, nil
}
