package postgres

import (
	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	auditdomain "github.com/StephenQiu30/lanverse/backend/internal/audit/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	inbox "github.com/StephenQiu30/lanverse/backend/internal/infra/inbox/application"
)

func parseIdentityAudit(event application.OutboxEvent) (auditdomain.Record, error) {
	if event.ID == uuid.Nil {
		return auditdomain.Record{}, ErrInvalidUser
	}
	parsed, err := auditapp.NewIdentityActionParser().Parse(inbox.Record{
		Topic: event.Topic, Key: []byte(event.PartitionKey), Value: event.Payload,
	})
	if err != nil || parsed.ID != event.ID {
		return auditdomain.Record{}, ErrInvalidUser
	}
	return parsed, nil
}
