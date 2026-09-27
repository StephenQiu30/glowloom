package audit_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	auditapp "github.com/StephenQiu30/lanverse/backend/internal/audit/application"
	"github.com/StephenQiu30/lanverse/backend/internal/audit/domain"
)

type readStoreStub struct {
	listActor uuid.UUID
	listOrg   uuid.UUID
	filter    domain.Filter
	page      domain.Page
	record    domain.Record
	err       error
}

func (s *readStoreStub) ListForAdmin(_ context.Context, actorID, orgID uuid.UUID, filter domain.Filter) (domain.Page, error) {
	s.listActor, s.listOrg, s.filter = actorID, orgID, filter
	return s.page, s.err
}

func (s *readStoreStub) GetForAdmin(_ context.Context, _, _, _ uuid.UUID) (domain.Record, error) {
	return s.record, s.err
}

func TestReadQueryValidatesScopeAndPreservesStoreErrors(t *testing.T) {
	actorID, orgID, recordID := uuid.New(), uuid.New(), uuid.New()
	store := &readStoreStub{page: domain.Page{Items: []domain.Record{{ID: recordID}}}}
	query := auditapp.NewReadQuery(store)
	now := time.Now()

	page, err := query.List(t.Context(), actorID, orgID, domain.Filter{})
	if err != nil || len(page.Items) != 1 || store.listActor != actorID || store.listOrg != orgID || store.filter.Limit != 50 {
		t.Fatalf("bounded list = %+v, filter = %+v, error = %v", page, store.filter, err)
	}
	for _, ids := range [][2]uuid.UUID{{uuid.Nil, orgID}, {actorID, uuid.Nil}} {
		if _, err := query.List(t.Context(), ids[0], ids[1], domain.Filter{}); !errors.Is(err, auditapp.ErrForbidden) {
			t.Fatalf("invalid actor scope: %v", err)
		}
	}
	for _, filter := range []domain.Filter{
		{Limit: 101},
		{From: now, To: now},
		{Before: &domain.Cursor{ID: uuid.New()}},
	} {
		if _, err := query.List(t.Context(), actorID, orgID, filter); !errors.Is(err, auditapp.ErrInvalidRead) {
			t.Fatalf("invalid filter %+v: %v", filter, err)
		}
	}
	if _, err := query.Get(t.Context(), actorID, orgID, uuid.Nil); !errors.Is(err, auditapp.ErrInvalidRead) {
		t.Fatalf("missing record ID: %v", err)
	}

	denied := errors.New("current role revoked")
	store.err = denied
	if _, err := query.List(t.Context(), actorID, orgID, domain.Filter{}); !errors.Is(err, denied) {
		t.Fatalf("list error chain: %v", err)
	}
	if _, err := query.Get(t.Context(), actorID, orgID, recordID); !errors.Is(err, denied) {
		t.Fatalf("detail error chain: %v", err)
	}
}
