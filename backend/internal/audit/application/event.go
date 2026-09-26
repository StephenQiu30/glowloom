// Package application validates audit events before durable consumption.
package application

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/audit/domain"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
)

const auditTopic = "lanverse.audit.recorded.v1"

// ErrInvalidEvent means an audit event cannot be stored without violating its contract.
var ErrInvalidEvent = errors.New("invalid audit event")

// Parser rejects actions and summary fields not explicitly declared by callers.
type Parser struct {
	fields map[string]map[string]struct{}
}

// NewParser constructs the action-specific summary field policy.
func NewParser(allowedFields map[string][]string) *Parser {
	fields := make(map[string]map[string]struct{}, len(allowedFields))
	for action, names := range allowedFields {
		set := make(map[string]struct{}, len(names))
		for _, name := range names {
			set[name] = struct{}{}
		}
		fields[action] = set
	}
	return &Parser{fields: fields}
}

// Parse validates routing, identity, action policy, and bounded summaries.
func (p *Parser) Parse(record inbox.Record) (domain.Record, error) {
	if record.Topic != auditTopic || len(record.Value) == 0 || len(record.Value) > 32*1024 {
		return domain.Record{}, fmt.Errorf("%w: topic or payload size", ErrInvalidEvent)
	}
	var body struct {
		EventID    string    `json:"event_id"`
		EventType  string    `json:"event_type"`
		OccurredAt time.Time `json:"occurred_at"`
		OrgID      string    `json:"org_id"`
		ProjectID  string    `json:"project_id"`
		Actor      struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
		} `json:"actor"`
		Aggregate struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		} `json:"aggregate"`
		Trace struct {
			Traceparent string `json:"traceparent"`
		} `json:"trace"`
		Data struct {
			Action string `json:"action"`
			Object struct {
				Type string `json:"type"`
				ID   string `json:"id"`
			} `json:"object"`
			Before    json.RawMessage `json:"before"`
			After     json.RawMessage `json:"after"`
			RequestID string          `json:"request_id"`
			IP        string          `json:"ip"`
		} `json:"data"`
	}
	if err := json.Unmarshal(record.Value, &body); err != nil {
		return domain.Record{}, fmt.Errorf("%w: decode: %w", ErrInvalidEvent, err)
	}
	if body.EventType != auditTopic || body.OccurredAt.IsZero() ||
		body.Aggregate.Type != "audit" || body.Aggregate.ID != body.EventID {
		return domain.Record{}, fmt.Errorf("%w: event envelope mismatch", ErrInvalidEvent)
	}
	id, err := uuid.Parse(body.EventID)
	if err != nil {
		return domain.Record{}, fmt.Errorf("%w: event ID: %w", ErrInvalidEvent, err)
	}
	orgID, err := uuid.Parse(body.OrgID)
	if err != nil {
		return domain.Record{}, fmt.Errorf("%w: organization ID: %w", ErrInvalidEvent, err)
	}
	result := domain.Record{ID: id, OrgID: orgID, CreateTime: body.OccurredAt.UTC()}
	key := body.OrgID
	if body.ProjectID != "" {
		projectID, err := uuid.Parse(body.ProjectID)
		if err != nil {
			return domain.Record{}, fmt.Errorf("%w: project ID: %w", ErrInvalidEvent, err)
		}
		result.ProjectID = &projectID
		key = body.ProjectID
	}
	if string(record.Key) != key {
		return domain.Record{}, fmt.Errorf("%w: partition key mismatch", ErrInvalidEvent)
	}
	if body.Actor.Kind != "user" && body.Actor.Kind != "agent" && body.Actor.Kind != "system" {
		return domain.Record{}, fmt.Errorf("%w: actor kind", ErrInvalidEvent)
	}
	if body.Actor.ID != "" {
		actorID, err := uuid.Parse(body.Actor.ID)
		if err != nil {
			return domain.Record{}, fmt.Errorf("%w: actor ID: %w", ErrInvalidEvent, err)
		}
		result.ActorID = &actorID
	} else if body.Actor.Kind != "system" {
		return domain.Record{}, fmt.Errorf("%w: actor ID required", ErrInvalidEvent)
	}
	allowed, ok := p.fields[body.Data.Action]
	if !ok || body.Data.Object.Type == "" || body.Data.Object.ID == "" ||
		len(body.Data.Object.Type) > 128 || len(body.Data.Object.ID) > 128 || len(body.Data.RequestID) > 128 {
		return domain.Record{}, fmt.Errorf("%w: action, object, or request ID", ErrInvalidEvent)
	}
	before, err := validateSummary(body.Data.Before, allowed)
	if err != nil {
		return domain.Record{}, err
	}
	after, err := validateSummary(body.Data.After, allowed)
	if err != nil {
		return domain.Record{}, err
	}
	if body.Data.IP != "" && net.ParseIP(body.Data.IP) == nil {
		return domain.Record{}, fmt.Errorf("%w: IP address", ErrInvalidEvent)
	}
	traceID, err := parseTraceID(body.Trace.Traceparent)
	if err != nil {
		return domain.Record{}, err
	}
	result.ActorKind = body.Actor.Kind
	result.Action = body.Data.Action
	result.ObjectType = body.Data.Object.Type
	result.ObjectID = body.Data.Object.ID
	result.Before = before
	result.After = after
	result.RequestID = body.Data.RequestID
	result.TraceID = traceID
	result.IP = body.Data.IP
	return result, nil
}

func validateSummary(raw json.RawMessage, allowed map[string]struct{}) (json.RawMessage, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	if len(raw) > 4*1024 {
		return nil, fmt.Errorf("%w: audit summary exceeds 4 KiB", ErrInvalidEvent)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil || len(fields) > 32 {
		return nil, fmt.Errorf("%w: audit summary is not a bounded object", ErrInvalidEvent)
	}
	for name, value := range fields {
		if _, ok := allowed[name]; !ok || unsafeSummaryField(name) {
			return nil, fmt.Errorf("%w: undeclared audit summary field %q", ErrInvalidEvent, name)
		}
		var scalar any
		if err := json.Unmarshal(value, &scalar); err != nil {
			return nil, fmt.Errorf("%w: summary value: %w", ErrInvalidEvent, err)
		}
		switch v := scalar.(type) {
		case nil, bool, float64:
		case string:
			lower := strings.ToLower(v)
			if !utf8.ValidString(v) || utf8.RuneCountInString(v) > 500 ||
				strings.Contains(lower, "signature=") || strings.Contains(lower, "awsaccesskeyid=") {
				return nil, fmt.Errorf("%w: unsafe audit summary text", ErrInvalidEvent)
			}
		default:
			return nil, fmt.Errorf("%w: nested audit summary", ErrInvalidEvent)
		}
	}
	return raw, nil
}

func unsafeSummaryField(name string) bool {
	canonical := strings.ToLower(strings.Map(func(r rune) rune {
		if r == '_' || r == '-' {
			return -1
		}
		return r
	}, name))
	switch canonical {
	case "password", "token", "secret", "secretkey", "apikey", "accesskey", "privatekey",
		"authorization", "credential", "credentials", "scripttext", "fullscript", "content":
		return true
	}
	return canonical == "url" || strings.HasSuffix(canonical, "url") ||
		strings.HasSuffix(canonical, "password") || strings.HasSuffix(canonical, "secret") ||
		strings.HasSuffix(canonical, "token") || strings.HasSuffix(canonical, "secretkey") ||
		strings.HasSuffix(canonical, "apikey") || strings.HasSuffix(canonical, "privatekey") ||
		strings.HasSuffix(canonical, "accesskey")
}

func parseTraceID(traceparent string) (string, error) {
	if traceparent == "" {
		return "", nil
	}
	parts := strings.Split(traceparent, "-")
	if len(parts) != 4 || parts[0] != "00" || len(parts[1]) != 32 || len(parts[2]) != 16 || len(parts[3]) != 2 {
		return "", fmt.Errorf("%w: traceparent format", ErrInvalidEvent)
	}
	for _, part := range parts {
		if _, err := hex.DecodeString(part); err != nil {
			return "", fmt.Errorf("%w: traceparent hex: %w", ErrInvalidEvent, err)
		}
	}
	if strings.Trim(parts[1], "0") == "" || strings.Trim(parts[2], "0") == "" {
		return "", fmt.Errorf("%w: empty trace or span ID", ErrInvalidEvent)
	}
	return strings.ToLower(parts[1]), nil
}
