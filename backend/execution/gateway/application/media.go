package application

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
)

func prepareMedia(c domain.Call, raw json.RawMessage) (domain.Call, error) {
	var in domain.MediaInput
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&in) != nil || dec.Decode(new(any)) != io.EOF || in.Model != c.Model.Name || in.Capability != c.Model.Capability || !slices.Contains(c.Model.Modes, in.Mode) || in.OutputCount != 1 || strings.TrimSpace(in.Prompt) == "" || len(in.Prompt) > 65536 || len(in.References) > 16 {
		return c, domain.ErrInvalid
	}
	if in.Params == nil {
		in.Params = map[string]any{}
	}
	for name, p := range c.Model.Parameters {
		if _, ok := in.Params[name]; !ok && len(p.Default) > 0 {
			var v any
			if json.Unmarshal(p.Default, &v) != nil {
				return c, domain.ErrInvalid
			}
			in.Params[name] = v
		}
	}
	for name, v := range in.Params {
		p, ok := c.Model.Parameters[name]
		if !ok || validateScalar(p, v) != nil {
			return c, domain.ErrInvalid
		}
	}
	for _, ref := range in.References {
		if ref.ID == uuid.Nil || ref.Role == "" || len(ref.Role) > 64 || len(ref.Version) > 160 {
			return c, domain.ErrInvalid
		}
	}
	if c.Project == uuid.Nil && len(in.References) > 0 {
		return c, domain.ErrInvalid
	}
	encodedRaw, _ := json.Marshal(in)
	c.Raw, _ = domain.CanonicalJSON(encodedRaw)
	hashInput := struct {
		Scope, Actor, Project, Run uuid.UUID
		Origin                     string
		Raw                        json.RawMessage
	}{c.Scope, c.Actor, c.Project, c.Run, c.Origin, c.Raw}
	encoded, _ := json.Marshal(hashInput)
	c.Hash = domain.Digest(encoded)
	var bound domain.Usage
	if c.Model.VideoMetering != nil {
		if err := c.Model.ValidateVideoInput(c.Project, in); err != nil {
			return c, err
		}
		var err error
		c.EstimatedVideoTokens, err = c.Model.EstimateVideo(in)
		if err != nil {
			return c, err
		}
		bound.Output = c.Model.MaxOutput
	}
	amount, err := c.Model.Price.Cost(bound)
	if err != nil {
		return c, err
	}
	c.Limit, err = domain.Credits(amount)
	return c, err
}
func validateScalar(p domain.Parameter, v any) error {
	switch p.Type {
	case "string":
		s, ok := v.(string)
		if !ok || len(s) > 10000 {
			return domain.ErrInvalid
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return domain.ErrInvalid
		}
	case "integer", "number":
		n, ok := v.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || math.Abs(n) > 1<<40 || p.Type == "integer" && math.Trunc(n) != n || p.Min != nil && n < *p.Min || p.Max != nil && n > *p.Max {
			return domain.ErrInvalid
		}
	default:
		return domain.ErrInvalid
	}
	if len(p.Enum) > 0 {
		actual, _ := json.Marshal(v)
		found := false
		for _, e := range p.Enum {
			var x any
			if json.Unmarshal(e, &x) != nil {
				return domain.ErrInvalid
			}
			b, _ := json.Marshal(x)
			if bytes.Equal(actual, b) {
				found = true
			}
		}
		if !found {
			return domain.ErrInvalid
		}
	}
	return nil
}
