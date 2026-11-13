// Package domain defines creation metrics from confirmed, actor-owned execution facts.
package domain

import (
	"errors"
	"net/url"
	"sort"
	"time"

	// Embed the fixed analysis timezone for minimal runtime images.
	_ "time/tzdata"

	"github.com/google/uuid"
)

// ErrInvalidFilter rejects unsupported and unbounded analysis queries.
var ErrInvalidFilter = errors.New("invalid analytics filter")

// Filter selects the accepted-operation cohort, bounded to 31 days.
type Filter struct {
	Scope              string
	From, To           time.Time
	ProjectID, ModelID *uuid.UUID
	Origin             string
}

// ParseFilter rejects duplicates and accepts only facts supported by the projection.
func ParseFilter(raw string, now time.Time) (Filter, error) {
	f := Filter{Scope: "personal", From: now.UTC().Add(-7 * 24 * time.Hour), To: now.UTC()}
	if len(raw) > 2048 {
		return f, ErrInvalidFilter
	}
	q, err := url.ParseQuery(raw)
	if err != nil {
		return f, ErrInvalidFilter
	}
	for key, items := range q {
		if len(items) != 1 || items[0] == "" {
			return f, ErrInvalidFilter
		}
		v := items[0]
		switch key {
		case "scope":
			f.Scope = v
		case "from", "to":
			parsed, e := time.Parse(time.RFC3339Nano, v)
			if e != nil {
				return f, ErrInvalidFilter
			}
			if key == "from" {
				f.From = parsed.UTC()
			} else {
				f.To = parsed.UTC()
			}
		case "project_id", "model_id":
			id, e := uuid.Parse(v)
			if e != nil || id == uuid.Nil || id.String() != v {
				return f, ErrInvalidFilter
			}
			if key == "project_id" {
				f.ProjectID = &id
			} else {
				f.ModelID = &id
			}
		case "origin":
			f.Origin = v
		default:
			return f, ErrInvalidFilter
		}
	}
	return f, f.Validate(now)
}

// Validate also protects non-HTTP callers from widening the read.
func (f Filter) Validate(now time.Time) error {
	if f.Scope != "personal" && f.Scope != "platform" {
		return ErrInvalidFilter
	}
	if f.From.IsZero() || !f.From.Before(f.To) || f.To.Sub(f.From) > 31*24*time.Hour || f.To.After(now.Add(time.Second)) {
		return ErrInvalidFilter
	}
	if (f.ProjectID != nil && *f.ProjectID == uuid.Nil) || (f.ModelID != nil && *f.ModelID == uuid.Nil) {
		return ErrInvalidFilter
	}
	if f.Origin != "" && f.Origin != "canvas" && f.Origin != "agent" && f.Origin != "pipeline" && f.Origin != "batch" && f.Origin != "system" {
		return ErrInvalidFilter
	}
	return nil
}

// Fact is one accepted operation; retry calls and outputs have separate counts.
type Fact struct {
	ModelName         *string
	ID                uuid.UUID
	AcceptedAt        time.Time
	Status            string
	ModelID           *uuid.UUID
	Origin            string
	Attempts, Outputs int64
}

// Capability explains why an unsupported metric has no numeric value.
type Capability struct {
	Key    string `json:"key" binding:"required"`
	Status string `json:"status" binding:"required"`
	Reason string `json:"reason" binding:"required"`
}

// Coverage describes the source and bounded current-state coverage.
type Coverage struct {
	Status        string       `json:"status" binding:"required"`
	Source        string       `json:"source" binding:"required"`
	MaxOperations int          `json:"max_operations" binding:"required"`
	Truncated     bool         `json:"truncated" binding:"required"`
	Capabilities  []Capability `json:"capabilities" binding:"required"`
}

// Count is a literal status or entry breakdown.
type Count struct {
	Key        string `json:"key" binding:"required"`
	Operations int64  `json:"operations" binding:"required"`
}

// ModelCount identifies immutable model versions without exposing credentials or prompts.
type ModelCount struct {
	ModelName  *string    `json:"model_name" binding:"required" extensions:"x-nullable"`
	ModelID    *uuid.UUID `json:"model_id" binding:"required" extensions:"x-nullable"`
	Operations int64      `json:"operations" binding:"required"`
}

// Day is an Asia/Shanghai accepted-cohort daily count with current terminal outcomes.
type Day struct {
	Date      string `json:"date" binding:"required"`
	Accepted  int64  `json:"accepted" binding:"required"`
	Completed int64  `json:"completed" binding:"required"`
	Failed    int64  `json:"failed" binding:"required"`
	Cancelled int64  `json:"cancelled" binding:"required"`
	Unknown   int64  `json:"unknown" binding:"required"`
}

// Creation is the read projection; unavailable money is null, never a fabricated zero.
type Creation struct {
	Timezone            string       `json:"timezone" binding:"required"`
	Scope               string       `json:"scope" binding:"required"`
	From                time.Time    `json:"from" binding:"required"`
	To                  time.Time    `json:"to" binding:"required"`
	AsOf                time.Time    `json:"as_of" binding:"required"`
	AcceptedOperations  int64        `json:"accepted_operations" binding:"required"`
	CompletedOperations int64        `json:"completed_operations" binding:"required"`
	FailedOperations    int64        `json:"failed_operations" binding:"required"`
	CancelledOperations int64        `json:"cancelled_operations" binding:"required"`
	UnknownOperations   int64        `json:"unknown_operations" binding:"required"`
	PartialOperations   int64        `json:"partial_operations" binding:"required"`
	Attempts            int64        `json:"attempts" binding:"required"`
	Outputs             int64        `json:"outputs" binding:"required"`
	TerminalSuccessRate *float64     `json:"terminal_success_rate" binding:"required" extensions:"x-nullable"`
	Daily               []Day        `json:"daily" binding:"required"`
	Statuses            []Count      `json:"statuses" binding:"required"`
	Entries             []Count      `json:"entries" binding:"required"`
	Models              []ModelCount `json:"models" binding:"required"`
	CostMicros          *string      `json:"cost_micros" binding:"required" extensions:"x-nullable"`
	RevenueMicros       *string      `json:"revenue_micros" binding:"required" extensions:"x-nullable"`
	ProfitMicros        *string      `json:"profit_micros" binding:"required" extensions:"x-nullable"`
	Coverage            Coverage     `json:"coverage" binding:"required"`
}

// Project deduplicates operation identities and never upgrades partial/unknown to success.
func Project(facts []Fact, f Filter, asOf time.Time, truncated bool) Creation {
	p := Creation{Timezone: "Asia/Shanghai", Scope: f.Scope, From: f.From, To: f.To, AsOf: asOf, Daily: []Day{}, Statuses: []Count{}, Entries: []Count{}, Models: []ModelCount{}, Coverage: Coverage{Status: "partial", Source: "operation.current_state", MaxOperations: 10000, Truncated: truncated, Capabilities: []Capability{
		{"accepted_generation", "available", "confirmed_at and confirmed_by; drafts, quotes and uploads excluded"},
		{"attempts", "available", "distinct dispatched submit attempt per operation; query/callback excluded"},
		{"outputs", "available", "nondeleted operation_output records; not adoption or delivered assets"},
		{"historical_status", "unavailable", "current state only; status history is not reconstructed"},
		{"adoption_delivery_retention", "unavailable", "no integrated immutable adoption, delivery or cohort projection"},
		{"visitors_registration_conversion", "unavailable", "no visitor collection or attributable conversion source"},
		{"cost_revenue_profit", "unavailable", "independent finance authority and reconciled financial projection unavailable"},
		{"team_scope", "unavailable", "separate team aggregation is not connected; personal counts only caller confirmed_by facts in accessible scopes"},
		{"projectless_execution", "unavailable", "canonical operation requires project; independent creation carrier projects are included"},
	}}}
	zone, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic("embedded analysis timezone unavailable")
	}
	fromLocal := f.From.In(zone)
	days := map[string]int{}
	for d := time.Date(fromLocal.Year(), fromLocal.Month(), fromLocal.Day(), 0, 0, 0, 0, zone); d.Before(f.To); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		days[key] = len(p.Daily)
		p.Daily = append(p.Daily, Day{Date: key})
	}
	statuses, entries, models := map[string]int64{}, map[string]int64{}, map[string]int64{}
	modelNames := map[string]*string{}
	seen := map[uuid.UUID]bool{}
	var terminal int64
	for _, fact := range facts {
		if fact.ID == uuid.Nil || seen[fact.ID] || fact.AcceptedAt.Before(f.From) || !fact.AcceptedAt.Before(f.To) || fact.Status == "draft" || fact.Status == "quoted" || fact.Status == "expired" || fact.Origin == "upload" {
			continue
		}
		seen[fact.ID] = true
		p.AcceptedOperations++
		p.Attempts += fact.Attempts
		p.Outputs += fact.Outputs
		statuses[fact.Status]++
		entries[fact.Origin]++
		key := ""
		if fact.ModelID != nil {
			key = fact.ModelID.String()
		}
		models[key]++
		modelNames[key] = fact.ModelName
		idx, exists := days[fact.AcceptedAt.In(zone).Format("2006-01-02")]
		if !exists {
			continue
		}
		day := &p.Daily[idx]
		day.Accepted++
		switch fact.Status {
		case "completed":
			p.CompletedOperations++
			day.Completed++
		case "failed":
			p.FailedOperations++
			day.Failed++
		case "cancelled":
			p.CancelledOperations++
			day.Cancelled++
		case "unknown", "reconciling", "manual":
			p.UnknownOperations++
			day.Unknown++
		}
		if fact.Outputs > 0 && fact.Status != "completed" {
			p.PartialOperations++
		} else if fact.Status == "completed" || fact.Status == "failed" || fact.Status == "cancelled" {
			terminal++
		}
	}
	denominator := terminal
	if denominator > 0 {
		rate := float64(p.CompletedOperations) / float64(denominator)
		p.TerminalSuccessRate = &rate
	}
	for key, n := range statuses {
		p.Statuses = append(p.Statuses, Count{key, n})
	}
	for key, n := range entries {
		p.Entries = append(p.Entries, Count{key, n})
	}
	for key, n := range models {
		var id *uuid.UUID
		if key != "" {
			v := uuid.MustParse(key)
			id = &v
		}
		p.Models = append(p.Models, ModelCount{ModelName: modelNames[key], ModelID: id, Operations: n})
	}
	sort.Slice(p.Statuses, func(i, j int) bool { return p.Statuses[i].Key < p.Statuses[j].Key })
	sort.Slice(p.Entries, func(i, j int) bool { return p.Entries[i].Key < p.Entries[j].Key })
	sort.Slice(p.Models, func(i, j int) bool {
		a, b := p.Models[i], p.Models[j]
		if a.ModelID == nil {
			return b.ModelID != nil
		}
		if b.ModelID == nil {
			return false
		}
		return a.ModelID.String() < b.ModelID.String()
	})
	return p
}

// Business exposes capability metadata only; platform governance grants no finance authority.
type Business struct {
	AsOf          time.Time    `json:"as_of" binding:"required"`
	Status        string       `json:"status" binding:"required"`
	Reason        string       `json:"reason" binding:"required"`
	RevenueMicros *string      `json:"revenue_micros" binding:"required" extensions:"x-nullable"`
	RefundMicros  *string      `json:"refund_micros" binding:"required" extensions:"x-nullable"`
	CostMicros    *string      `json:"cost_micros" binding:"required" extensions:"x-nullable"`
	ProfitMicros  *string      `json:"profit_micros" binding:"required" extensions:"x-nullable"`
	Capabilities  []Capability `json:"capabilities" binding:"required"`
}

// UnavailableBusiness carries no procurement, payment or wallet data.
func UnavailableBusiness(now time.Time) Business {
	return Business{AsOf: now.UTC(), Status: "unavailable", Reason: "finance_authorization_not_connected", Capabilities: []Capability{{"payments_membership", "unavailable", "verified payment and membership receipts not connected"}, {"costs_profit", "unavailable", "independent finance permission and reconciled cost allocation not connected"}}}
}
