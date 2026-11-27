package http

import (
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/httpapi"
)

type taskPageToken struct {
	ID         uuid.UUID `json:"id"`
	CreateTime time.Time `json:"create_time"`
	Binding    string    `json:"binding"`
}

type taskPagination struct {
	binding string
}

func newTaskPagination(filter any) (taskPagination, error) {
	binding, err := httpapi.CursorBinding(filter)
	return taskPagination{binding: binding}, err
}

func (p taskPagination) after(token string) (*application.TaskCursor, error) {
	if token == "" {
		return nil, nil
	}
	var cursor taskPageToken
	if len(token) > 1024 || !httpapi.DecodeCursorToken(token, &cursor) || cursor.Binding != p.binding || cursor.ID == uuid.Nil || cursor.CreateTime.IsZero() {
		return nil, application.ErrInvalidPublicQuery
	}
	return &application.TaskCursor{ID: cursor.ID, CreateTime: cursor.CreateTime}, nil
}

func (p taskPagination) next(cursor *application.TaskCursor) (*string, error) {
	if cursor == nil {
		return nil, nil
	}
	return httpapi.EncodeCursorToken(taskPageToken{ID: cursor.ID, CreateTime: cursor.CreateTime, Binding: p.binding})
}
