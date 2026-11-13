// Package domain defines the safe diagnostic projection and bounded queries.
package domain

import (
	"encoding/base64"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidFilter rejects ambiguous or unbounded diagnostic queries.
var ErrInvalidFilter = errors.New("invalid diagnostic filter")

// ErrNotFound means the record is absent from this process retention window.
var ErrNotFound = errors.New("diagnostic record not found")
var tracePattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Filter uses UTC half-open bounds; cursor continuation restores omitted bounds.
type Filter struct {
	From, To                                   time.Time
	FromSet, ToSet                             bool
	Level, Service, RequestID, TraceID, Cursor string
	Limit                                      int
}

// ParseFilter accepts only explicit, unique, bounded whitelist fields.
func ParseFilter(raw string, now time.Time) (Filter, error) {
	f := Filter{From: now.UTC().Add(-24 * time.Hour), To: now.UTC(), Limit: 50}
	if len(raw) > 4096 {
		return f, ErrInvalidFilter
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return f, ErrInvalidFilter
	}
	for key, items := range values {
		if len(items) != 1 || items[0] == "" {
			return f, ErrInvalidFilter
		}
		v := items[0]
		switch key {
		case "from", "to":
			t, e := time.Parse(time.RFC3339Nano, v)
			if e != nil {
				return f, ErrInvalidFilter
			}
			if key == "from" {
				f.From = t.UTC()
				f.FromSet = true
			} else {
				f.To = t.UTC()
				f.ToSet = true
			}
		case "level":
			if v != "info" && v != "warn" && v != "error" {
				return f, ErrInvalidFilter
			}
			f.Level = v
		case "service":
			if v != "backend" {
				return f, ErrInvalidFilter
			}
			f.Service = v
		case "request_id":
			id, e := uuid.Parse(v)
			if e != nil || id == uuid.Nil || id.String() != v {
				return f, ErrInvalidFilter
			}
			f.RequestID = v
		case "trace_id":
			if !tracePattern.MatchString(v) || v == "00000000000000000000000000000000" {
				return f, ErrInvalidFilter
			}
			f.TraceID = v
		case "limit":
			n, e := strconv.Atoi(v)
			if e != nil || n < 1 || n > 100 {
				return f, ErrInvalidFilter
			}
			f.Limit = n
		case "cursor":
			if len(v) > 2048 {
				return f, ErrInvalidFilter
			}
			if data, e := base64.RawURLEncoding.DecodeString(v); e != nil || len(data) < 33 {
				return f, ErrInvalidFilter
			}
			f.Cursor = v
		default:
			return f, ErrInvalidFilter
		}
	}
	if err := f.Validate(now); err != nil {
		return f, err
	}
	return f, nil
}

// Validate checks public fields. A signed cursor supplies any omitted dates
// before the source validates the complete time window.
func (f Filter) Validate(now time.Time) error {
	if f.Limit < 1 || f.Limit > 100 {
		return ErrInvalidFilter
	}
	if f.Level != "" && f.Level != "info" && f.Level != "warn" && f.Level != "error" {
		return ErrInvalidFilter
	}
	if f.Service != "" && f.Service != "backend" {
		return ErrInvalidFilter
	}
	if f.Cursor != "" && (!f.FromSet || !f.ToSet) {
		return nil
	}
	if f.From.IsZero() || !f.From.Before(f.To) || f.To.Sub(f.From) > 7*24*time.Hour || f.To.After(now.Add(time.Second)) {
		return ErrInvalidFilter
	}
	return nil
}

// DiagnosticRecord contains only server-controlled request completion metadata.
type DiagnosticRecord struct {
	Panic      bool      `json:"panic" binding:"required"`
	ID         uuid.UUID `json:"id" binding:"required"`
	Timestamp  time.Time `json:"timestamp" binding:"required"`
	Level      string    `json:"level" binding:"required"`
	Service    string    `json:"service" binding:"required"`
	Role       string    `json:"role" binding:"required"`
	Release    string    `json:"release" binding:"required"`
	Event      string    `json:"event" binding:"required"`
	Method     string    `json:"method" binding:"required"`
	Route      string    `json:"route" binding:"required"`
	Status     int       `json:"status" binding:"required"`
	ElapsedMS  float64   `json:"elapsed_ms" binding:"required"`
	RequestID  string    `json:"request_id" binding:"required"`
	TraceID    string    `json:"trace_id,omitempty"`
	ErrorClass string    `json:"error_class,omitempty"`
}

// Coverage makes restart loss, eviction and absent services explicit.
type Coverage struct {
	Status           string     `json:"status" binding:"required"`
	Source           string     `json:"source" binding:"required"`
	StartedAt        time.Time  `json:"started_at" binding:"required"`
	OldestAt         *time.Time `json:"oldest_at" binding:"required" extensions:"x-nullable"`
	RetentionSeconds int64      `json:"retention_seconds" binding:"required"`
	Capacity         int        `json:"capacity" binding:"required"`
	Retained         int        `json:"retained" binding:"required"`
	Evicted          uint64     `json:"evicted" binding:"required"`
	RestartLoss      bool       `json:"restart_loss" binding:"required"`
	MissingServices  []string   `json:"missing_services" binding:"required"`
}

// Page is a stable bounded cursor page; continuation fails after snapshot eviction.
type Page struct {
	FirstCursor string             `json:"first_cursor" binding:"required"`
	Items       []DiagnosticRecord `json:"items" binding:"required"`
	NextCursor  *string            `json:"next_cursor" binding:"required" extensions:"x-nullable"`
	AsOf        time.Time          `json:"as_of" binding:"required"`
	From        time.Time          `json:"from" binding:"required"`
	To          time.Time          `json:"to" binding:"required"`
	Coverage    Coverage           `json:"coverage" binding:"required"`
}

// Overview reports the same retained request population as the filtered list.
type Overview struct {
	Panics          int       `json:"panics" binding:"required"`
	AsOf            time.Time `json:"as_of" binding:"required"`
	From            time.Time `json:"from" binding:"required"`
	To              time.Time `json:"to" binding:"required"`
	Requests        int       `json:"requests" binding:"required"`
	ServerErrors    int       `json:"server_errors" binding:"required"`
	ClientErrors    int       `json:"client_errors" binding:"required"`
	RateLimited     int       `json:"rate_limited" binding:"required"`
	ServerErrorRate *float64  `json:"server_error_rate" binding:"required" extensions:"x-nullable"`
	P50MS           *float64  `json:"p50_ms" binding:"required" extensions:"x-nullable"`
	P95MS           *float64  `json:"p95_ms" binding:"required" extensions:"x-nullable"`
	Coverage        Coverage  `json:"coverage" binding:"required"`
	Unavailable     []string  `json:"unavailable" binding:"required"`
}

// Detail contains no raw exception, headers, request contents or business content.
type Detail struct {
	Record   DiagnosticRecord `json:"record" binding:"required"`
	Coverage Coverage         `json:"coverage" binding:"required"`
}
