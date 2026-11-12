package domain

// CallKind identifies the owner of an Agent business invocation.
type CallKind string

// Call kinds match agent_business.call_receipt.kind.
const (
	CallKindModel = "model"
	CallKindTool  = "tool"
)

// CallStatus is the persisted outcome of one original invocation.
type CallStatus string

// Call statuses match agent_business.call_receipt.status.
const (
	CallStatusInFlight  = "in_flight"
	CallStatusUnknown   = "unknown"
	CallStatusCompleted = "completed"
	CallStatusFailed    = "failed"
)
