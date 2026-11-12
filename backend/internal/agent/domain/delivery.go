package domain

// DeliveryStatus describes authoritative business output, independently of the
// model submission state. These values are shared by tools and run snapshots.
type DeliveryStatus string

// Delivery states describe the original artifacts, never model output text.
const (
	DeliveryPending        DeliveryStatus = "pending"
	DeliveryAwaitingReview DeliveryStatus = "awaiting_review"
	DeliveryIncomplete     DeliveryStatus = "incomplete"
	DeliveryCompleted      DeliveryStatus = "completed"
)
