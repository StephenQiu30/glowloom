package domain

import "time"

const (
	MetricGenerationImageCall = "generation.image.call"
	MetricGenerationVideoCall = "generation.video.call"
	WindowUTCDay              = "UTC_DAY"

	ReservationReserved = "RESERVED"
	ReservationConsumed = "CONSUMED"
	ReservationReleased = "RELEASED"
)

func IsGenerationMetric(value string) bool {
	return value == MetricGenerationImageCall || value == MetricGenerationVideoCall
}

type Policy struct {
	ID, WorkspaceID, ProjectID, Metric, WindowKind string
	LimitUnits, Revision                           int64
	ContentHash, CreatedBy, UpdatedBy              string
	CreatedAt, UpdatedAt                           time.Time
}

type Counter struct {
	ID, WorkspaceID, ProjectID, PolicyID, Metric string
	WindowStart, WindowEnd                       time.Time
	PolicyRevision, LimitUnits                   int64
	ReservedUnits, ConsumedUnits, Revision       int64
	CreatedAt, UpdatedAt                         time.Time
}

type Reservation struct {
	ID, WorkspaceID, ProjectID, PolicyID, CounterID string
	Metric, SourceType, SourceID, Status            string
	WindowStart, WindowEnd                          time.Time
	PolicyRevision, LimitUnits, Units, Revision     int64
	BindingHash, CreatedBy                          string
	CreatedAt, UpdatedAt                            time.Time
	ConsumedAt, ReleasedAt                          *time.Time
}

type Usage struct {
	PolicyID, CounterID, WorkspaceID, ProjectID, Metric string
	WindowStart, WindowEnd                              time.Time
	PolicyRevision, LimitUnits                          int64
	ReservedUnits, ConsumedUnits, AvailableUnits        int64
}

func DailyWindow(value time.Time) (time.Time, time.Time) {
	utc := value.UTC()
	start := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
	return start, start.Add(24 * time.Hour)
}

func SamePolicyState(left, right Policy) bool {
	return left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.Metric == right.Metric && left.WindowKind == right.WindowKind &&
		left.LimitUnits == right.LimitUnits && left.Revision == right.Revision &&
		left.ContentHash == right.ContentHash
}

func SameReservationBinding(left, right Reservation) bool {
	return left.WorkspaceID == right.WorkspaceID && left.ProjectID == right.ProjectID &&
		left.PolicyID == right.PolicyID && left.CounterID == right.CounterID && left.Metric == right.Metric &&
		left.SourceType == right.SourceType && left.SourceID == right.SourceID &&
		left.WindowStart.Equal(right.WindowStart) && left.WindowEnd.Equal(right.WindowEnd) &&
		left.PolicyRevision == right.PolicyRevision && left.LimitUnits == right.LimitUnits &&
		left.Units == right.Units && left.BindingHash == right.BindingHash
}
