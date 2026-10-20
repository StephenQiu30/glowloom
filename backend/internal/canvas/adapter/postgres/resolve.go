package postgres

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// Resolve reads an actor-bound save receipt under current read authority, including
// archived projects. The advisory lock waits out any identical in-flight save.
func (s *Store) Resolve(ctx context.Context, a identity.Principal, id uuid.UUID, key string, input application.CommandsInput) (*application.Result, error) {
	var result application.Result
	var found bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		project, err := documentProject(tx, id)
		if err != nil {
			return err
		}
		if err = s.authorize(tx, a, project, false); err != nil {
			return err
		}
		body, err := json.Marshal(input)
		if err != nil {
			return err
		}
		found, err = replaySave(tx, a.ID, id, key, requestHash("commands/"+id.String(), body), &result)
		return err
	})
	if err != nil || !found {
		return nil, err
	}
	return &result, nil
}
