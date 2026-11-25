package domain

// CommandStatus is the durable lifecycle of one script source command.
type CommandStatus string

// Command states distinguish recoverable writes from completed or cancelled commands.
const (
	CommandStatusPending   CommandStatus = "pending"
	CommandStatusCompleted CommandStatus = "completed"
	CommandStatusCancelled CommandStatus = "cancelled"
)

// CommandIOState describes ownership and proven exit of remote object I/O.
type CommandIOState string

// Command I/O states record ownership and proven cessation of object operations.
const (
	CommandIOStateIdle    CommandIOState = "idle"
	CommandIOStateRunning CommandIOState = "running"
	CommandIOStateEnded   CommandIOState = "ended"
	CommandIOStateUnknown CommandIOState = "unknown"
)
