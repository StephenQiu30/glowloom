package domain

import (
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	mediatool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

// GoalKind identifies the admitted business obligation, independently of model text.
type GoalKind string

// Goal kinds distinguish conversation from an admitted production obligation.
const (
	GoalChat       GoalKind = "chat"
	GoalProduction GoalKind = "production"
)

// MaxGoalDeliveries bounds admitted requirements and their shot receipt projection.
const MaxGoalDeliveries = 64

// MaxProductionOperations bounds the original operations exposed for root recovery.
const MaxProductionOperations = 64

// GoalDeliverySpec freezes optional single-shot output requirements.
type GoalDeliverySpec struct {
	DurationMS    int64                `json:"duration_ms"`
	SourceStartMS int64                `json:"source_start_ms"`
	AspectRatio   string               `json:"aspect_ratio"`
	FPS           int                  `json:"fps"`
	ProjectFormat mediatool.OutputKind `json:"project_format"`
}

// TimelineConfig constructs the shared single-shot timeline defaults used by
// both goal validation and the actual delivery's adopted timeline.
func (s GoalDeliverySpec) TimelineConfig() canvas.TimelineConfig {
	return canvas.TimelineConfig{Version: 1, AspectRatio: s.AspectRatio, FPS: s.FPS, SubtitleStyle: canvas.SubtitleStyle{FontSize: 24, Color: "#ffffff", Position: "bottom"}}
}

// Valid reuses the timeline's output rules and the supported project formats.
func (s GoalDeliverySpec) Valid() bool {
	timeline := s.TimelineConfig()
	return s.SourceStartMS >= 0 && s.DurationMS >= canvas.MinTimelineClipDurationMS && s.DurationMS <= canvas.MaxTimelineDurationMS && s.SourceStartMS <= canvas.MaxTimelineDurationMS-s.DurationMS && (s.ProjectFormat == mediatool.OutputFCPXML || s.ProjectFormat == mediatool.OutputXMEML || s.ProjectFormat == mediatool.OutputJianying) && timeline.Validate() == nil
}

// GoalContract is frozen by Go at admission. Missing production source remains a
// planning obligation; it never means that an empty delivery set is complete.
type GoalContract struct {
	Kind               GoalKind                `json:"kind"`
	RequiredDeliveries int                     `json:"required_deliveries"`
	Source             *operation.CanvasSource `json:"source,omitempty"`
	Delivery           *GoalDeliverySpec       `json:"delivery,omitempty"`
}

// NormalizeGoalContract applies business defaults and validates supported scope.
func NormalizeGoalContract(in GoalContract, scope Scope) (GoalContract, error) {
	if in.Kind == "" {
		in.Kind = GoalChat
	}
	switch in.Kind {
	case GoalChat:
		if in.RequiredDeliveries != 0 || in.Source != nil || in.Delivery != nil {
			return GoalContract{}, ErrInvalid
		}
	case GoalProduction:
		if in.RequiredDeliveries == 0 {
			in.RequiredDeliveries = 1
		}
		if in.RequiredDeliveries < 1 || in.RequiredDeliveries > MaxGoalDeliveries {
			return GoalContract{}, ErrInvalid
		}
		if in.Source != nil {
			if !in.Source.Valid() || in.Source.RowID != nil {
				return GoalContract{}, ErrInvalid
			}
			if scope.CanvasID != nil && *scope.CanvasID != in.Source.CanvasID {
				return GoalContract{}, ErrConflict
			}
		}
		if in.Delivery != nil && !in.Delivery.Valid() {
			return GoalContract{}, ErrInvalid
		}
	default:
		return GoalContract{}, ErrInvalid
	}
	return in, nil
}

// GoalCompletion reports current original-job facts against the frozen goal.
type GoalCompletion struct {
	ContractHash        string         `json:"contract_hash"`
	Kind                GoalKind       `json:"kind"`
	Status              DeliveryStatus `json:"status"`
	RequiredDeliveries  int            `json:"required_deliveries"`
	CompletedDeliveries int            `json:"completed_deliveries"`
	Missing             []string       `json:"missing"`
}
