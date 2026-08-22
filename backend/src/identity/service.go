package identity

import (
	"context"

	"github.com/google/uuid"

	"github.com/stephenqiu30/lanverse/backend/src/platform/httpapi"
)

type IdentityService struct{ repository IdentityStore }

func NewIdentityService(repository IdentityStore) *IdentityService {
	return &IdentityService{repository: repository}
}

func (s *IdentityService) CreateSession(ctx context.Context, subject string, workspaceID uuid.UUID) (Session, error) {
	if subject == "" || workspaceID == uuid.Nil {
		return Session{}, httpapi.Validation("身份主体和 Workspace 不能为空", "提供有效的身份主体和 Workspace 后重试")
	}
	return s.repository.CreateSession(ctx, subject, workspaceID)
}

func (s *IdentityService) Authenticate(ctx context.Context, token string, workspaceID uuid.UUID) (Principal, error) {
	if token == "" || workspaceID == uuid.Nil {
		return Principal{}, httpapi.Validation("会话和 Workspace 不能为空", "提供有效的会话和 Workspace 后重试")
	}
	return s.repository.Authenticate(ctx, token, workspaceID)
}

func (s *IdentityService) Revoke(ctx context.Context, token string) error {
	return s.repository.Revoke(ctx, token)
}

func (s *IdentityService) AuthorizePath(ctx context.Context, workspaceID uuid.UUID, path string) error {
	return s.repository.AuthorizePath(ctx, workspaceID, path)
}
