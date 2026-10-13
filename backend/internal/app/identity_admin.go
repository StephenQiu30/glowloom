package app

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// adminAccountStore translates the legacy duplicate-login sentinel at composition.
// Account transactions, password hashing and database schema remain unchanged.
type adminAccountStore struct{ *pgidentity.Store }

func (s adminAccountStore) CreateWithEvents(ctx context.Context, actor uuid.UUID, user domain.User, events []application.OutboxEvent) (domain.User, error) {
	result, err := s.Store.CreateWithEvents(ctx, actor, user, events)
	if errors.Is(err, pgidentity.ErrLoginExists) {
		return domain.User{}, application.ErrLoginTaken
	}
	return result, err
}
func provideIdentityAdmin(database *gorm.DB) *identityhttp.AdminHandler {
	store := pgidentity.NewStore(database)
	return identityhttp.NewAdminHandler(identityhttp.AdminDependencies{List: application.NewListUsersQuery(store), Create: application.NewCreateUserCommand(adminAccountStore{store}, time.Now), Update: application.NewUpdateUserCommand(store, time.Now), Disable: application.NewDisableUserCommand(store, time.Now), Enable: application.NewEnableUserCommand(store, time.Now), Reset: application.NewResetPasswordCommand(store, time.Now), Unlock: application.NewUnlockUserCommand(store, time.Now)})
}
