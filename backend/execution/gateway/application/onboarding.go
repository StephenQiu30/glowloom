package application

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/execution/gateway/domain"
)

// OnboardingOrigin identifies authorized test calls in the shared request ledger.
const OnboardingOrigin = "onboarding"

// Onboarding is the immutable, keyless route and test plan reviewed before confirmation.
type Onboarding struct {
	Route       domain.Route      `json:"route"`
	Scope       uuid.UUID         `json:"scope_id"`
	Project     uuid.UUID         `json:"project_id"`
	Replacement uuid.UUID         `json:"replaces_route_id"`
	MediaTests  []json.RawMessage `json:"media_tests,omitempty"`
	Concurrency int               `json:"concurrency"`
	RPM         int               `json:"rpm"`
}

// Probe identifies one independently metered supplier invocation.
type Probe struct {
	Name string          `json:"name"`
	Raw  json.RawMessage `json:"input" swaggertype:"object"`
	Tool bool            `json:"tool,omitempty"`
}

// OnboardingPlan is the plan shown to a payer before uploading a key.
type OnboardingPlan struct {
	Probes         []Probe `json:"probes"`
	Credits        int64   `json:"credits"`
	PurchaseMicros int64   `json:"purchase_micros"`
}

// PurchaseBound uses the full reviewed limits, including the most expensive input bucket.
func PurchaseBound(m domain.Model, p domain.Price) (int64, error) {
	if m.Protocol == "media" && !m.AcceptsMediaPrice(p) {
		return 0, domain.ErrInvalid
	}
	p.Input = max(p.Input, p.CacheRead, p.CacheWrite)
	return p.Cost(domain.Usage{Input: m.MaxInput, Output: m.MaxOutput})
}

// PlanOnboarding constructs probes from the actual enabled product contract.
func PlanOnboarding(in Onboarding, model domain.Model) (OnboardingPlan, error) {
	var plan OnboardingPlan
	if model.Validate() != nil || in.Scope == uuid.Nil || in.Route.ModelID != model.ID {
		return plan, domain.ErrInvalid
	}
	if model.Protocol == "media" {
		if in.Project == uuid.Nil || len(in.MediaTests) == 0 || len(in.MediaTests) != len(model.Modes) || len(model.Modes) > 16 {
			return plan, domain.ErrInvalid
		}
		seen := make(map[string]bool, len(in.MediaTests))
		for _, raw := range in.MediaTests {
			var m domain.MediaInput
			if len(raw) > 64<<10 || json.Unmarshal(raw, &m) != nil || seen[m.Mode] || !slices.Contains(model.Modes, m.Mode) {
				return plan, domain.ErrInvalid
			}
			// Test inputs are already versioned assets in the authorized test project.
			for _, ref := range m.References {
				if ref.Version == "" {
					return plan, domain.ErrInvalid
				}
			}
			plan.Probes = append(plan.Probes, Probe{Name: m.Mode, Raw: raw})
			seen[m.Mode] = true
		}
	} else {
		if len(in.MediaTests) != 0 {
			return plan, domain.ErrInvalid
		}
		if model.Protocol == domain.EmbeddingProtocol {
			raw, _ := json.Marshal(EmbeddingInput{Model: model.Name, Text: "项目镜头检索", Purpose: "query"})
			plan.Probes = []Probe{{Name: "文本向量", Raw: raw}}
		} else {
			plan.Probes = []Probe{textProbe(model, false, false)}
		}
		if model.Stream {
			plan.Probes = append(plan.Probes, textProbe(model, true, false))
		}
		if model.Tools {
			plan.Probes = append(plan.Probes, textProbe(model, false, true))
			if model.Stream {
				plan.Probes = append(plan.Probes, textProbe(model, true, true))
			}
		}
	}
	for i := range plan.Probes {
		call, err := OnboardingCall(in, model, uuid.Nil, plan.Probes[i], i)
		if err != nil || call.Limit > math.MaxInt64-plan.Credits {
			return plan, domain.ErrInvalid
		}
		plan.Probes[i].Raw = call.Raw
		plan.Credits += call.Limit
	}
	cost, err := PurchaseBound(model, in.Route.Purchase)
	if err != nil || cost > model.PurchaseCap || cost > math.MaxInt64/int64(len(plan.Probes)) {
		return plan, domain.ErrBudget
	}
	plan.PurchaseMicros = cost * int64(len(plan.Probes))
	return plan, nil
}

func textProbe(m domain.Model, stream, tool bool) Probe {
	name := "非流式"
	if stream {
		name = "流式"
	}
	prompt := "Reply with OK."
	if tool {
		name += "工具调用"
		prompt = "Call the verify tool with an empty object."
	}
	maxOutput := min(m.MaxOutput, 128)
	fields := map[string]any{"model": m.Name, "stream": stream, "max_tokens": maxOutput,
		"messages": []map[string]string{{"role": "user", "content": prompt}}}
	spec := map[string]any{"type": "object", "properties": map[string]any{}}
	switch m.Protocol {
	case "chat":
		if tool {
			fields["tools"] = []any{map[string]any{"type": "function", "function": map[string]any{"name": "verify", "parameters": spec}}}
		}
	case "messages":
		if tool {
			fields["tools"] = []any{map[string]any{"name": "verify", "input_schema": spec}}
		}
	case "responses":
		delete(fields, "messages")
		delete(fields, "max_tokens")
		fields["input"], fields["max_output_tokens"] = prompt, maxOutput
		if tool {
			fields["tools"] = []any{map[string]any{"type": "function", "name": "verify", "parameters": spec}}
		}
	}
	if tool {
		switch m.Protocol {
		case "messages":
			fields["tool_choice"] = map[string]string{"type": "tool", "name": "verify"}
		case "responses":
			fields["tool_choice"] = map[string]string{"type": "function", "name": "verify"}
		default:
			fields["tool_choice"] = map[string]any{"type": "function", "function": map[string]string{"name": "verify"}}
		}
	}
	raw, _ := json.Marshal(fields)
	return Probe{Name: name, Raw: raw, Tool: tool}
}

// OnboardingCall derives a stable dispatch and spending identity for every probe.
func OnboardingCall(in Onboarding, m domain.Model, actor uuid.UUID, probe Probe, index int) (domain.Call, error) {
	key := fmt.Sprintf("onboarding/%s/%d", in.Route.ID, index)
	call, err := Prepare(m, actor, in.Scope, in.Project, in.Route.ID, key, OnboardingOrigin, probe.Raw)
	call.ID = uuid.NewSHA1(in.Route.ID, []byte(key))
	return call, err
}

// ProbePassed requires generation evidence, not a manual evidence-only settlement.
func ProbePassed(protocol string, probe Probe, receipt *domain.Receipt) bool {
	if receipt == nil || receipt.Pending {
		return false
	}
	if protocol == domain.EmbeddingProtocol {
		_, err := domain.EmbeddingVector(receipt.Body)
		return err == nil && receipt.Usage.Input > 0
	}
	var body struct {
		ReconciliationOnly bool `json:"reconciliation_only"`
		Choices            []struct {
			Message struct {
				Content string `json:"content"`
				Tools   []struct {
					Function struct{ Name string } `json:"function"`
				} `json:"tool_calls"`
			}
		}
		Output []struct {
			Type, Name string
			Content    []struct{ Type, Text string }
		}
		Content []struct{ Type, Name, Text string }
	}
	if json.Unmarshal(receipt.Body, &body) != nil || body.ReconciliationOnly {
		return false
	}
	hasText := func(text string) bool { return strings.TrimSpace(text) != "" }
	switch protocol {
	case "media":
		// The worker separately requires delivery of the verified bytes.
		return true
	case "chat":
		if len(body.Choices) != 1 {
			return false
		}
		message := body.Choices[0].Message
		if !probe.Tool {
			return hasText(message.Content)
		}
		return len(message.Tools) == 1 && message.Tools[0].Function.Name == "verify"
	case "messages":
		for _, block := range body.Content {
			if probe.Tool && block.Type == "tool_use" && block.Name == "verify" ||
				!probe.Tool && block.Type == "text" && hasText(block.Text) {
				return true
			}
		}
	case "responses":
		for _, item := range body.Output {
			if probe.Tool && item.Type == "function_call" && item.Name == "verify" {
				return true
			}
			for _, content := range item.Content {
				if !probe.Tool && item.Type == "message" && content.Type == "output_text" && hasText(content.Text) {
					return true
				}
			}
		}
	}
	return false
}
