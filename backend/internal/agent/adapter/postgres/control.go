package postgres

import (
	"context"
	"encoding/json"
	"time"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ControlStore owns Go authorization facts. It never invokes a Host in a transaction.
type ControlStore struct {
	db     *gorm.DB
	access func(*gorm.DB) app.ControlAccess
}

func NewControlStore(db *gorm.DB, access func(*gorm.DB) app.ControlAccess) *ControlStore {
	return &ControlStore{db: db, access: access}
}
func (s *ControlStore) authorize(tx *gorm.DB, actor identity.Principal, pid uuid.UUID, write bool) error {
	if s.access == nil || actor.ID == uuid.Nil || pid == uuid.Nil {
		return identity.ErrForbidden
	}
	return s.access(tx).AuthorizeRequest(tx.Statement.Context, actor, pid, write)
}
func (s *ControlStore) CreateSession(ctx context.Context, actor identity.Principal, pid, key uuid.UUID, title string) (domain.Session, error) {
	var result domain.Session
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(tx, actor, pid, true); err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO agent.session(id,project_id,actor_id,request_id,title,engine) VALUES(?,?,?,?,?,?) ON CONFLICT(project_id,actor_id,request_id) DO NOTHING`, uuid.New(), pid, actor.ID, key, title, domain.HarnessEngine).Error; err != nil {
			return err
		}
		var row sessionRow
		r := tx.Raw(`SELECT id,project_id,actor_id,title,engine,revision,updated_at,turns FROM agent.session WHERE project_id=? AND actor_id=? AND request_id=?`, pid, actor.ID, key).Scan(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 || row.Title != title || row.Engine != domain.HarnessEngine {
			return domain.ErrConflict
		}
		result = domain.Session{ID: row.ID, ProjectID: row.ProjectID, ActorID: row.ActorID, Title: row.Title, Engine: row.Engine, Revision: row.Revision, UpdatedAt: row.UpdatedAt, Turns: []domain.Turn{}}
		return nil
	})
	return result, err
}
func controlSession(tx *gorm.DB, actor identity.Principal, pid, sid uuid.UUID) (int64, error) {
	var row struct {
		Revision int64
		Engine   string
	}
	q := tx.Raw(`SELECT revision,engine FROM agent.session WHERE id=? AND project_id=? AND actor_id=? FOR UPDATE`, sid, pid, actor.ID).Scan(&row)
	if q.Error != nil {
		return 0, q.Error
	}
	if q.RowsAffected != 1 {
		return 0, domain.ErrNotFound
	}
	if row.Engine != domain.HarnessEngine {
		return 0, domain.ErrConflict
	}
	return row.Revision, nil
}
func findStart(tx *gorm.DB, sid, key uuid.UUID, hash string) (app.RunReceipt, bool, error) {
	var row struct {
		InputHash string
		Receipt   []byte
	}
	q := tx.Raw(`SELECT r.input_hash,c.receipt FROM agent.harness_run r JOIN agent.harness_command c ON c.run_id=r.id AND c.action='start' WHERE r.session_id=? AND r.request_id=?`, sid, key).Scan(&row)
	if q.Error != nil {
		return app.RunReceipt{}, false, q.Error
	}
	if q.RowsAffected == 0 {
		return app.RunReceipt{}, false, nil
	}
	if row.InputHash != hash {
		return app.RunReceipt{}, false, domain.ErrConflict
	}
	var receipt app.RunReceipt
	if json.Unmarshal(row.Receipt, &receipt) != nil {
		return receipt, false, domain.ErrInvalid
	}
	return receipt, true, nil
}
func (s *ControlStore) FindStart(ctx context.Context, actor identity.Principal, pid, sid, key uuid.UUID, hash string) (app.RunReceipt, bool, error) {
	var receipt app.RunReceipt
	var found bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(tx, actor, pid, true); err != nil {
			return err
		}
		if _, err := controlSession(tx, actor, pid, sid); err != nil {
			return err
		}
		var err error
		receipt, found, err = findStart(tx, sid, key, hash)
		return err
	})
	return receipt, found, err
}
func runReceipt(run domain.Run, id uuid.UUID) app.RunReceipt {
	return app.RunReceipt{RunID: run.ID, CommandID: id, Revision: run.Revision, ControlRevision: run.ControlRevision, Status: run.Status, CandidateID: run.Candidate.ID}
}
func insertCandidate(tx *gorm.DB, r domain.Run, key uuid.UUID) error {
	c := r.Candidate
	return tx.Exec(`INSERT INTO agent.harness_candidate(id,run_id,request_id,purpose,content_hash,manifest,plan_revision) VALUES(?,?,?,?,?,?::jsonb,?)`, c.ID, r.ID, key, c.Purpose, c.Hash, string(c.Manifest), c.PlanRevision).Error
}
func writeControlEvent(tx *gorm.DB, run domain.Run, receipt app.RunReceipt) error {
	payload, err := json.Marshal(map[string]any{"event_id": receipt.CommandID, "event_type": "glowloom.agent.command.v1", "data": receipt, "project_id": run.ProjectID})
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,'glowloom.agent.command.v1',?,?::jsonb)`, receipt.CommandID, run.ProjectID.String(), string(payload)).Error
}
func insertCommand(tx *gorm.DB, r domain.Run, key uuid.UUID, hash, action string, delivery app.RuntimeDelivery) (app.RunReceipt, error) {
	receipt := runReceipt(r, uuid.New())
	raw, err := json.Marshal(receipt)
	if err != nil {
		return receipt, err
	}
	body, err := json.Marshal(delivery)
	if err != nil {
		return receipt, err
	}
	state := "pending"
	if delivery.Kind == "none" {
		state = "acknowledged"
	}
	if err := tx.Exec(`INSERT INTO agent.harness_command(id,run_id,request_id,input_hash,action,receipt,delivery,delivery_state) VALUES(?,?,?,?,?,?::jsonb,?::jsonb,?)`, receipt.CommandID, r.ID, key, hash, action, string(raw), string(body), state).Error; err != nil {
		return receipt, err
	}
	return receipt, writeControlEvent(tx, r, receipt)
}
func (s *ControlStore) CreateRun(ctx context.Context, actor identity.Principal, run domain.Run, expected int64, hash string) (app.RunReceipt, error) {
	var receipt app.RunReceipt
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(tx, actor, run.ProjectID, true); err != nil {
			return err
		}
		revision, err := controlSession(tx, actor, run.ProjectID, run.SessionID)
		if err != nil {
			return err
		}
		if old, found, err := findStart(tx, run.SessionID, run.RequestID, hash); err != nil || found {
			receipt = old
			return err
		}
		if revision != expected {
			return domain.ErrConflict
		}
		var active bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM agent.harness_run WHERE session_id=? AND status NOT IN ('stopped','rejected','failed','completed','partially_completed'))`, run.SessionID).Scan(&active).Error; err != nil {
			return err
		}
		if active {
			return domain.ErrConflict
		}
		raw, err := json.Marshal(run)
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO agent.harness_run(id,session_id,project_id,actor_id,org_id,scope_id,runtime_session_id,request_id,input_hash,revision,control_revision,status,state) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?::jsonb)`, run.ID, run.SessionID, run.ProjectID, run.ActorID, run.OrgID, run.ScopeID, run.RuntimeSessionID, run.RequestID, hash, run.Revision, run.ControlRevision, run.Status, string(raw)).Error; err != nil {
			return err
		}
		if err := insertCandidate(tx, run, run.RequestID); err != nil {
			return err
		}
		receipt, err = insertCommand(tx, run, run.RequestID, hash, "start", app.RuntimeDelivery{Kind: "none"})
		if err != nil {
			return err
		}
		return tx.Exec(`UPDATE agent.session SET revision=revision+1,updated_at=now() WHERE id=?`, run.SessionID).Error
	})
	return receipt, err
}
func loadControlRun(tx *gorm.DB, rid uuid.UUID) (domain.Run, error) {
	var row struct {
		State                     []byte
		ActorID, OrgID, ScopeID   uuid.UUID
		Revision, ControlRevision int64
	}
	q := tx.Raw(`SELECT state,actor_id,org_id,scope_id,revision,control_revision FROM agent.harness_run WHERE id=? FOR UPDATE`, rid).Scan(&row)
	if q.Error != nil {
		return domain.Run{}, q.Error
	}
	if q.RowsAffected != 1 {
		return domain.Run{}, domain.ErrNotFound
	}
	var run domain.Run
	if json.Unmarshal(row.State, &run) != nil || run.ID != rid || run.Revision != row.Revision || run.ControlRevision != row.ControlRevision {
		return run, domain.ErrInvalid
	}
	run.ActorID = row.ActorID
	run.OrgID = row.OrgID
	run.ScopeID = row.ScopeID
	return run, nil
}
func saveControlRun(tx *gorm.DB, run domain.Run) error {
	raw, err := json.Marshal(run)
	if err != nil {
		return err
	}
	q := tx.Exec(`UPDATE agent.harness_run SET state=?::jsonb,revision=?,control_revision=?,status=? WHERE id=?`, string(raw), run.Revision, run.ControlRevision, run.Status, run.ID)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 {
		return domain.ErrNotFound
	}
	return nil
}
func (s *ControlStore) requestRun(tx *gorm.DB, actor identity.Principal, pid, rid uuid.UUID, write bool) (domain.Run, error) {
	run, err := loadControlRun(tx, rid)
	if err != nil {
		return run, err
	}
	if run.ProjectID != pid || run.ActorID != actor.ID {
		return domain.Run{}, domain.ErrNotFound
	}
	if err := s.authorize(tx, actor, pid, write); err != nil {
		return domain.Run{}, err
	}
	return run, nil
}
func (s *ControlStore) runtimeRun(tx *gorm.DB, id domain.RuntimeIdentity, now time.Time, action string) (domain.Run, error) {
	run, err := loadControlRun(tx, id.RunID)
	if err != nil {
		return run, err
	}
	if err := run.Authorize(id, "receipt.read", now); err != nil {
		return run, err
	}
	if action == "owner.drain" {
		return run, nil
	}
	if s.access == nil {
		return run, identity.ErrForbidden
	}
	actor := identity.Principal{ID: run.ActorID, OrgID: run.OrgID, ScopeID: run.ScopeID}
	if err := s.access(tx).AuthorizeWorker(tx.Statement.Context, actor, run.ProjectID); err != nil {
		return run, err
	}
	return run, nil
}
func (s *ControlStore) ReadRun(ctx context.Context, actor identity.Principal, pid, rid uuid.UUID) (domain.Run, error) {
	var run domain.Run
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		run, err = s.requestRun(tx, actor, pid, rid, false)
		return err
	})
	return run, err
}
func (s *ControlStore) ChangeRun(ctx context.Context, actor identity.Principal, pid, rid uuid.UUID, input app.ControlRequest, hash string, change app.Change) (app.RunReceipt, error) {
	var receipt app.RunReceipt
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		write := input.Action != "stop" && input.Action != "pause" && input.Action != "reject"
		run, err := s.requestRun(tx, actor, pid, rid, write)
		if err != nil {
			return err
		}
		var previous struct {
			InputHash string
			Receipt   []byte
		}
		q := tx.Raw(`SELECT input_hash,receipt FROM agent.harness_command WHERE run_id=? AND request_id=?`, rid, input.RequestID).Scan(&previous)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 1 {
			if previous.InputHash != hash {
				return domain.ErrConflict
			}
			return json.Unmarshal(previous.Receipt, &receipt)
		}
		if run.Revision != input.ExpectedRevision {
			return domain.ErrConflict
		}
		oldCandidate := run.Candidate.ID
		delivery, err := change(&run)
		if err != nil {
			return err
		}
		if oldCandidate != run.Candidate.ID {
			if err := insertCandidate(tx, run, input.RequestID); err != nil {
				return err
			}
		}
		// Superseded dispatches cannot open a new scope after a barrier; in-flight external effects retain their receipts.
		if err := tx.Exec(`UPDATE agent.harness_command SET delivery_state='superseded' WHERE run_id=? AND delivery_state IN ('pending','leased')`, rid).Error; err != nil {
			return err
		}
		if err := saveControlRun(tx, run); err != nil {
			return err
		}
		receipt, err = insertCommand(tx, run, input.RequestID, hash, input.Action, delivery)
		if err != nil {
			return err
		}
		if input.Action == "approve" {
			return tx.Exec(`INSERT INTO agent.harness_approval(id,run_id,candidate_id,actor_id,max_micros,expires_at) VALUES(?,?,?,?,?,?)`, receipt.CommandID, rid, run.Candidate.ID, actor.ID, run.ApprovalMaxMicros, run.ApprovalExpiresAt).Error
		}
		return nil
	})
	return receipt, err
}
func (s *ControlStore) RegisterPlan(ctx context.Context, id domain.RuntimeIdentity, key uuid.UUID, plan domain.PlanManifest, now time.Time) (domain.ApprovalCandidate, error) {
	var result domain.ApprovalCandidate
	hash, err := domain.CanonicalHash(plan)
	if err != nil {
		return result, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		run, err := s.runtimeRun(tx, id, now, "receipt.read")
		if err != nil {
			return err
		}
		var row struct {
			ID                   uuid.UUID
			Purpose, ContentHash string
			Manifest             []byte
			PlanRevision         int64
		}
		q := tx.Raw(`SELECT id,purpose,content_hash,manifest,plan_revision FROM agent.harness_candidate WHERE run_id=? AND request_id=?`, run.ID, key).Scan(&row)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 1 {
			if row.ContentHash != hash {
				return domain.ErrConflict
			}
			result = domain.ApprovalCandidate{ID: row.ID, Purpose: row.Purpose, Hash: row.ContentHash, Manifest: row.Manifest, PlanRevision: row.PlanRevision}
			return nil
		}
		if err := run.Authorize(id, "plan.register", now); err != nil {
			return err
		}
		result, err = run.RegisterPlan(uuid.New(), plan, now)
		if err != nil {
			return err
		}
		if err := insertCandidate(tx, run, key); err != nil {
			return err
		}
		if err := saveControlRun(tx, run); err != nil {
			return err
		}
		_, err = insertCommand(tx, run, key, hash, "plan.register", app.RuntimeDelivery{Kind: "none"})
		return err
	})
	return result, err
}
func (s *ControlStore) Authorize(ctx context.Context, id domain.RuntimeIdentity, action string, now time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		run, err := s.runtimeRun(tx, id, now, action)
		if err != nil {
			return err
		}
		return run.Authorize(id, action, now)
	})
}
func (s *ControlStore) Admit(ctx context.Context, id domain.RuntimeIdentity, key uuid.UUID, action domain.ActionRequest, now time.Time) (app.AdmissionReceipt, error) {
	var receipt app.AdmissionReceipt
	hash, err := domain.CanonicalHash(action)
	if err != nil {
		return receipt, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		run, err := s.runtimeRun(tx, id, now, "model.generate")
		if err != nil {
			return err
		}
		if err := run.Authorize(id, "model.generate", now); err != nil {
			return err
		}
		var row struct {
			ID                         uuid.UUID
			InputHash, Status          string
			ControlRevision, MaxMicros int64
		}
		q := tx.Raw(`SELECT id,input_hash,status,control_revision,max_micros FROM agent.harness_admission WHERE run_id=? AND request_id=?`, run.ID, key).Scan(&row)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 1 {
			if row.InputHash != hash {
				return domain.ErrConflict
			}
			receipt = app.AdmissionReceipt{ID: row.ID, RunID: run.ID, Status: row.Status, ControlRevision: row.ControlRevision, MaxMicros: row.MaxMicros, Replayed: true}
			return nil
		}
		if err := run.Reserve(action, now); err != nil {
			return err
		}
		receipt = app.AdmissionReceipt{ID: uuid.New(), RunID: run.ID, Status: "in_flight", ControlRevision: run.ControlRevision, MaxMicros: action.MaxMicros}
		if err := tx.Exec(`INSERT INTO agent.harness_admission(id,run_id,request_id,input_hash,action,candidate_id,control_revision,max_micros) VALUES(?,?,?,?,?,?,?,?)`, receipt.ID, run.ID, key, hash, action.Action, run.Candidate.ID, run.ControlRevision, action.MaxMicros).Error; err != nil {
			return err
		}
		return saveControlRun(tx, run)
	})
	return receipt, err
}

func (s *ControlStore) ClaimDelivery(ctx context.Context, now time.Time) (app.DeliveryClaim, bool, error) {
	var claim app.DeliveryClaim
	found := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct {
			ID, RunID uuid.UUID
			Delivery  []byte
			Attempts  int
		}
		q := tx.Raw(`
			SELECT c.id,c.run_id,c.delivery,c.attempts
			FROM agent.harness_command c
			WHERE ((c.delivery_state='pending' AND c.next_attempt_at<=?)
			   OR (c.delivery_state='leased' AND c.claim_until<=?))
			  AND NOT EXISTS (
				SELECT 1 FROM agent.harness_command earlier
				WHERE earlier.run_id=c.run_id
				  AND earlier.delivery_state IN ('pending','leased')
				  AND (earlier.created_at,earlier.id)<(c.created_at,c.id)
			  )
			ORDER BY c.created_at,c.id
			LIMIT 1 FOR UPDATE OF c SKIP LOCKED`, now, now).Scan(&row)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 0 {
			return nil
		}
		token := uuid.New()
		leaseUntil := now.Add(30 * time.Second)
		updated := tx.Exec(`UPDATE agent.harness_command SET delivery_state='leased',claim_token=?,claim_until=?,attempts=attempts+1 WHERE id=? AND delivery_state IN ('pending','leased')`, token, leaseUntil, row.ID)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return domain.ErrConflict
		}
		var delivery app.RuntimeDelivery
		if json.Unmarshal(row.Delivery, &delivery) != nil {
			return domain.ErrInvalid
		}
		claim = app.DeliveryClaim{ID: row.ID, Token: token, RunID: row.RunID, Attempt: row.Attempts + 1, Value: delivery}
		found = true
		return nil
	})
	return claim, found, err
}

func (s *ControlStore) CompleteDelivery(ctx context.Context, id, token uuid.UUID) error {
	q := s.db.WithContext(ctx).Exec(`UPDATE agent.harness_command SET delivery_state='acknowledged',claim_token=NULL,claim_until=NULL WHERE id=? AND claim_token=? AND delivery_state='leased'`, id, token)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 {
		return domain.ErrConflict
	}
	return nil
}

func (s *ControlStore) RetryDelivery(ctx context.Context, id, token uuid.UUID, next time.Time) error {
	q := s.db.WithContext(ctx).Exec(`UPDATE agent.harness_command SET delivery_state='pending',claim_token=NULL,claim_until=NULL,next_attempt_at=? WHERE id=? AND claim_token=? AND delivery_state='leased'`, next, id, token)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 {
		return domain.ErrConflict
	}
	return nil
}

var _ app.ControlStore = (*ControlStore)(nil)
