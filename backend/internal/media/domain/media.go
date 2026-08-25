package domain

import "time"

type UploadSession struct {
	ID, WorkspaceID, Status, Kind, Filename, MIMEType, SHA256, ObjectKey, IdempotencyKey string
	MediaObjectID                                                                        *string
	SizeBytes                                                                            int64
	ExpiresAt, CreatedAt, UpdatedAt                                                      time.Time
	CompletedAt                                                                          *time.Time
}

type MediaObject struct {
	ID, WorkspaceID, Kind, SourceType, Status string
	CurrentVersionID                          *string
	Revision                                  int
	CreatedAt, UpdatedAt                      time.Time
}

type MediaVersion struct {
	ID, WorkspaceID, MediaObjectID, Filename, SHA256, MIMEType, ObjectKey, ProbeStatus string
	VersionNo, ProbeAttempt                                                            int
	SizeBytes                                                                          int64
	ProbeErrorCode, ProbeSummary, ProbeNextAction, Codec, Container                    *string
	Width, Height, DurationMS                                                          *int
	CreatedAt                                                                          time.Time
	MediaObject                                                                        MediaObject
}

type Task struct {
	ID, WorkspaceID, TaskType, RequestType, RequestID, Status, ProgressStage, CancelStatus string
	Scope, Error                                                                           []byte
	NextAction                                                                             *string
	Revision                                                                               int
	CreatedAt, UpdatedAt                                                                   time.Time
}
