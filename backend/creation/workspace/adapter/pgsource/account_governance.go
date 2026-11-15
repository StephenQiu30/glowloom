package pgsource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	workspace "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

// AccountGovernanceStore owns Creation's management latch and retained effects.
// All source qualification RPCs occur in application before these SQL methods.
type AccountGovernanceStore struct{ db *gorm.DB }

// NewAccountGovernanceStore accepts only an actual Creation root capability.
func NewAccountGovernanceStore(db *gorm.DB) *AccountGovernanceStore {
	return &AccountGovernanceStore{db: db}
}
func (s *AccountGovernanceStore) transaction(ctx context.Context, run func(*gorm.DB) error) error {
	if s == nil || s.db == nil || s.db.Statement == nil {
		return writefence.ErrUnavailable
	}
	if _, open := s.db.Statement.ConnPool.(gorm.TxCommitter); open {
		return writefence.ErrTransaction
	}
	return taskruntime.Transaction(ctx, s.db, func(tx *gorm.DB) error {
		owner, err := pgfence.New(tx).Owner(ctx)
		if err != nil {
			return err
		}
		if owner != writefence.WriterCreation {
			return writefence.ErrUnavailable
		}
		return run(tx)
	})
}

type governanceSnapshot struct{ Teams []workspace.GovernanceTeam }
type governancePhase struct {
	Proofs         map[string]workspace.GovernanceQualification `json:",omitempty"`
	FreezeControls []writefence.Control                         `json:"freeze_controls,omitempty"`
}
type governanceRow struct {
	RequestID, AccountID                    uuid.UUID
	Action, InputSHA256, Verdict            string
	IdentityClose, Snapshot, Phase, Receipt json.RawMessage
	AcceptedAt                              time.Time
	EffectsAppliedAt, CompletedAt           *time.Time
}

func readAccountGovernance(tx *gorm.DB, id uuid.UUID, lock bool) (workspace.AccountGovernancePreparation, error) {
	query := `SELECT request_id,account_id,action,input_sha256,identity_close,snapshot,phase,receipt,verdict,accepted_at,effects_applied_at,completed_at FROM workspace.account_governance_preparation WHERE request_id=?`
	if lock {
		query += ` FOR UPDATE`
	}
	var row governanceRow
	read := tx.Raw(query, id).Scan(&row)
	if read.Error != nil {
		return workspace.AccountGovernancePreparation{}, read.Error
	}
	if read.RowsAffected != 1 {
		return workspace.AccountGovernancePreparation{}, writefence.ErrNotFound
	}
	var intent workspace.AccountGovernanceIntent
	var snapshot governanceSnapshot
	var phase governancePhase
	if json.Unmarshal(row.IdentityClose, &intent) != nil || json.Unmarshal(row.Snapshot, &snapshot) != nil || json.Unmarshal(row.Phase, &phase) != nil || intent.Validate() != nil || intent.RequestID != row.RequestID || intent.AccountID != row.AccountID || intent.Action != row.Action || intent.InputSHA256 != row.InputSHA256 {
		return workspace.AccountGovernancePreparation{}, writefence.ErrConflict
	}
	out := workspace.AccountGovernancePreparation{Intent: intent, Teams: snapshot.Teams, FreezeControls: phase.FreezeControls, Applied: row.EffectsAppliedAt != nil, AcceptedAt: row.AcceptedAt}
	keys := make([]string, 0, len(phase.Proofs))
	for key := range phase.Proofs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		proof := phase.Proofs[key]
		if proof.Validate() != nil || proof.Intent != intent {
			return out, writefence.ErrConflict
		}
		out.Proofs = append(out.Proofs, proof)
	}
	if row.Verdict != "pending" {
		var receipt workspace.AccountGovernanceReceipt
		if row.CompletedAt == nil || json.Unmarshal(row.Receipt, &receipt) != nil || !governanceReceiptMatches(receipt, intent) || receipt.RejectedDuties != (row.Verdict == "rejected_duties") {
			return out, writefence.ErrConflict
		}
		out.Receipt = &receipt
	}
	return out, nil
}

// LoadAccountGovernance never creates a responsibility or advances a fence.
func (s *AccountGovernanceStore) LoadAccountGovernance(ctx context.Context, id uuid.UUID) (workspace.AccountGovernancePreparation, error) {
	var out workspace.AccountGovernancePreparation
	err := s.transaction(ctx, func(tx *gorm.DB) error { var err error; out, err = readAccountGovernance(tx, id, false); return err })
	return out, err
}

// AcceptAccountGovernance serializes only relevant team/member management. It
// leaves every shared scope epoch and other member's admitted task unchanged.
func (s *AccountGovernanceStore) AcceptAccountGovernance(ctx context.Context, intent workspace.AccountGovernanceIntent) (workspace.AccountGovernancePreparation, error) {
	if intent.Validate() != nil {
		return workspace.AccountGovernancePreparation{}, writefence.ErrInvalid
	}
	var out workspace.AccountGovernancePreparation
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if err := lockTeamGovernanceManagement(tx); err != nil {
			return err
		}
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "creation.account_governance/"+intent.RequestID.String()).Error; err != nil {
			return err
		}
		if old, err := readAccountGovernance(tx, intent.RequestID, true); err == nil {
			if old.Intent != intent {
				return writefence.ErrConflict
			}
			out = old
			return nil
		} else if !errors.Is(err, writefence.ErrNotFound) {
			return err
		}
		if err := lockGovernanceBlockedFence(tx, intent.CloseControl); err != nil {
			return err
		}
		teams, err := governanceTeams(tx, intent.AccountID)
		if err != nil {
			return err
		}
		for i := range teams {
			if err := requireNoTeamGovernancePending(tx, teams[i].ID, uuid.Nil); err != nil {
				return err
			}
			teams[i].SnapshotSHA256 = governanceTeamDigest(intent, teams[i])
		}
		body, err := json.Marshal(intent)
		if err != nil {
			return err
		}
		snapshot, err := json.Marshal(governanceSnapshot{Teams: teams})
		if err != nil {
			return err
		}
		var now time.Time
		if err := tx.Raw(`SELECT statement_timestamp()`).Scan(&now).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO workspace.account_governance_preparation(request_id,account_id,action,input_sha256,identity_close,snapshot,accepted_at) VALUES(?,?,?,?,?::jsonb,?::jsonb,?)`, intent.RequestID, intent.AccountID, intent.Action, intent.InputSHA256, string(body), string(snapshot), now).Error; err != nil {
			return err
		}
		if err := taskruntime.Enqueue(ctx, tx, workspace.AccountGovernanceRecoveryTask, intent.RequestID.String(), now); err != nil {
			return err
		}
		out = workspace.AccountGovernancePreparation{Intent: intent, Teams: teams, AcceptedAt: now}
		return nil
	})
	return out, err
}

// SaveGovernanceQualification records immutable purpose evidence. Positive
// admission is local and outside the policy transaction; no RPC is hidden in SQL.
func (s *AccountGovernanceStore) SaveGovernanceQualification(ctx context.Context, p workspace.AccountGovernancePreparation, proof workspace.GovernanceQualification) error {
	if proof.Validate() != nil || proof.Intent != p.Intent {
		return writefence.ErrInvalid
	}
	if proof.Eligible {
		owner, err := pgfence.New(s.db).Owner(ctx)
		if err != nil {
			return err
		}
		if owner != writefence.WriterCreation {
			return writefence.ErrUnavailable
		}
		record, err := pgfence.New(s.db).Admit(ctx, proof.Command)
		if err != nil {
			return err
		}
		if record.State == writefence.Frozen {
			return writefence.ErrFenced
		}
	}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		current, err := readAccountGovernance(tx, p.Intent.RequestID, true)
		if err != nil {
			return err
		}
		if current.Intent != p.Intent || current.Applied || current.Receipt != nil {
			return writefence.ErrConflict
		}
		if !governanceCandidateBound(current, proof) {
			return writefence.ErrInvalid
		}
		phase := governancePhase{Proofs: map[string]workspace.GovernanceQualification{}, FreezeControls: current.FreezeControls}
		for _, old := range current.Proofs {
			phase.Proofs[old.ID.String()] = old
		}
		if old, ok := phase.Proofs[proof.ID.String()]; ok {
			oldJSON, _ := json.Marshal(old)
			newJSON, _ := json.Marshal(proof)
			if string(oldJSON) != string(newJSON) {
				return writefence.ErrConflict
			}
			return nil
		}
		phase.Proofs[proof.ID.String()] = proof
		encoded, err := json.Marshal(phase)
		if err != nil {
			return err
		}
		return tx.Exec(`UPDATE workspace.account_governance_preparation SET phase=?::jsonb WHERE request_id=? AND verdict='pending'`, string(encoded), p.Intent.RequestID).Error
	})
}

// ApplyAccountGovernance is the final account-candidate/team/member policy point.
// Permanent freeze and its source-close decision commit together. Rejection only
// records the original duties receipt; it never installs a shared-scope control.
func (s *AccountGovernanceStore) ApplyAccountGovernance(ctx context.Context, p workspace.AccountGovernancePreparation, proofs []workspace.GovernanceQualification) (workspace.AccountGovernancePreparation, error) {
	var out workspace.AccountGovernancePreparation
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if err := lockTeamGovernanceManagement(tx); err != nil {
			return err
		}
		if err := tx.Exec(`SELECT set_config('glowloom.creation_governance_request',?,true)`, p.Intent.RequestID.String()).Error; err != nil {
			return err
		}
		if err := lockGovernanceBlockedFence(tx, p.Intent.CloseControl); err != nil {
			return err
		}
		ordered := append([]workspace.GovernanceQualification(nil), proofs...)
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].Control.Key() < ordered[j].Control.Key() })
		for _, proof := range ordered {
			if proof.Validate() != nil || proof.Intent != p.Intent || !governanceCandidateBound(p, proof) {
				return writefence.ErrInvalid
			}
			if proof.Eligible {
				if err := pgfence.New(s.db).Guard(ctx, tx, proof.Command); err != nil {
					return err
				}
			} else if err := lockGovernanceBlockedFence(tx, proof.Control); err != nil {
				return err
			}
		}
		current, err := readAccountGovernance(tx, p.Intent.RequestID, true)
		if err != nil {
			return err
		}
		if current.Intent != p.Intent {
			return writefence.ErrConflict
		}
		if current.Receipt != nil || current.Applied {
			out = current
			return nil
		}
		teams, err := governanceTeams(tx, p.Intent.AccountID)
		if err != nil {
			return err
		}
		if !sameGovernanceTeams(p.Intent, current.Teams, teams) {
			return writefence.ErrConflict
		}
		for _, proof := range ordered {
			found := false
			for _, saved := range current.Proofs {
				if saved.ID == proof.ID && saved.Command.SHA256 == proof.Command.SHA256 {
					found = true
				}
			}
			if !found {
				return writefence.ErrInvalid
			}
		}
		receipt := workspace.AccountGovernanceReceipt{RequestID: p.Intent.RequestID, AccountID: p.Intent.AccountID, Action: p.Intent.Action, InputSHA256: p.Intent.InputSHA256, IdentityClose: p.Intent.CloseControl, Teams: []uuid.UUID{}, BlockingTeams: []uuid.UUID{}}
		freeze := map[uuid.UUID]bool{}
		for _, team := range current.Teams {
			receipt.Teams = append(receipt.Teams, team.ID)
			targetAdmin := team.Target.Role == "team_admin" && team.Target.Status == "active"
			if p.Intent.Action == "delete" && !targetAdmin {
				continue
			}
			if p.Intent.Action != "delete" && team.GovernanceBlocked {
				continue
			}
			available := p.Intent.Action == "enable" && p.Intent.EligibleAfter && targetAdmin
			if !available {
				for _, member := range team.Candidates {
					matched := false
					for _, proof := range ordered {
						if proof.TeamID == team.ID && proof.CandidateID == member.ID {
							matched = true
							if proof.Eligible {
								available = true
							}
							break
						}
					}
					if available {
						break
					}
					if !matched {
						return writefence.ErrUnavailable
					}
				}
			}
			if !available {
				if p.Intent.Action == "delete" {
					receipt.BlockingTeams = append(receipt.BlockingTeams, team.ID)
				} else {
					freeze[team.ID] = true
				}
			}
		}
		if len(receipt.BlockingTeams) > 0 {
			receipt.RejectedDuties = true
			if err := finishGovernanceReceipt(tx, current, receipt, "rejected_duties"); err != nil {
				return err
			}
			out = current
			out.Receipt = &receipt
			return nil
		}
		phase := governancePhase{Proofs: map[string]workspace.GovernanceQualification{}}
		for _, proof := range current.Proofs {
			phase.Proofs[proof.ID.String()] = proof
		}
		for _, team := range current.Teams {
			if freeze[team.ID] {
				var head struct {
					Epoch    int64
					Complete bool
				}
				read := tx.Raw(`SELECT epoch,complete FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='creation' AND authority_kind='scope' AND authority_id=? FOR UPDATE`, team.ScopeID).Scan(&head)
				if read.Error != nil {
					return read.Error
				}
				if read.RowsAffected != 1 || !head.Complete {
					return writefence.ErrUnavailable
				}
				coord := uuid.NewSHA1(p.Intent.RequestID, []byte("creation.account_governance.freeze/"+team.ScopeID.String()))
				state, err := pgfence.NewAuthorityStore(tx).BeginTx(ctx, tx, writefence.Authority{Issuer: writefence.Creation, Kind: writefence.Scope, ID: team.ScopeID, Epoch: head.Epoch}, coord, true)
				if err != nil {
					return err
				}
				phase.FreezeControls = append(phase.FreezeControls, state.Control)
			}
			if err := tx.Exec(`UPDATE workspace.team SET governance_blocked=governance_blocked OR ?,auth_epoch=auth_epoch+1,revision=revision+1,update_time=statement_timestamp() WHERE id=? AND revision=?`, freeze[team.ID], team.ID, team.Revision).Error; err != nil {
				return err
			}
			if p.Intent.Action == "delete" {
				if err := tx.Exec(`UPDATE workspace.team_member SET status='removed',auth_epoch=auth_epoch+1,revision=revision+1,update_time=statement_timestamp() WHERE team_id=? AND user_id=? AND status<>'removed'`, team.ID, p.Intent.AccountID).Error; err != nil {
					return err
				}
			}
		}
		encoded, err := json.Marshal(phase)
		if err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE workspace.account_governance_preparation SET phase=?::jsonb,effects_applied_at=GREATEST(statement_timestamp(),accepted_at) WHERE request_id=?`, string(encoded), p.Intent.RequestID).Error; err != nil {
			return err
		}
		out = current
		out.Applied = true
		out.FreezeControls = phase.FreezeControls
		return nil
	})
	return out, err
}

// CompleteAccountGovernance verifies only actual committed effects and exact
// permanent-freeze four-writer receipts before exposing a ready resource receipt.
func (s *AccountGovernanceStore) CompleteAccountGovernance(ctx context.Context, p workspace.AccountGovernancePreparation) (workspace.AccountGovernanceReceipt, error) {
	var out workspace.AccountGovernanceReceipt
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if err := lockTeamGovernanceManagement(tx); err != nil {
			return err
		}
		current, err := readAccountGovernance(tx, p.Intent.RequestID, true)
		if err != nil {
			return err
		}
		if current.Intent != p.Intent {
			return writefence.ErrConflict
		}
		if current.Receipt != nil {
			out = *current.Receipt
			return nil
		}
		if !current.Applied {
			return writefence.ErrUnavailable
		}
		for _, control := range current.FreezeControls {
			var exact bool
			if err := tx.Raw(`SELECT epoch=? AND coordination_id=? AND blocked AND complete FROM infra.authority_head WHERE service_owner=infra.runtime_owner() AND issuer='creation' AND authority_kind='scope' AND authority_id=? FOR UPDATE`, control.Epoch, control.CoordinationID, control.ID).Scan(&exact).Error; err != nil {
				return err
			}
			if !exact {
				return writefence.ErrFenced
			}
			state, err := pgfence.NewAuthorityStore(tx).LoadCoordination(ctx, control.CoordinationID)
			if err != nil {
				return err
			}
			if !state.Complete || state.Control != control || len(state.Acknowledged) != 4 {
				return writefence.ErrUnavailable
			}
		}
		out = workspace.AccountGovernanceReceipt{RequestID: p.Intent.RequestID, AccountID: p.Intent.AccountID, Action: p.Intent.Action, InputSHA256: p.Intent.InputSHA256, IdentityClose: p.Intent.CloseControl, Teams: []uuid.UUID{}, BlockingTeams: []uuid.UUID{}}
		for _, team := range current.Teams {
			var row struct {
				Revision, AuthEpoch int64
				GovernanceBlocked   bool
			}
			read := tx.Raw(`SELECT revision,auth_epoch,governance_blocked FROM workspace.team WHERE id=? FOR UPDATE`, team.ID).Scan(&row)
			if read.Error != nil {
				return read.Error
			}
			if read.RowsAffected != 1 || row.Revision != team.Revision+1 || row.AuthEpoch != team.AuthEpoch+1 {
				return writefence.ErrConflict
			}
			for _, control := range current.FreezeControls {
				if control.ID == team.ScopeID && !row.GovernanceBlocked {
					return writefence.ErrConflict
				}
			}
			if p.Intent.Action == "delete" {
				var removed bool
				if err := tx.Raw(`SELECT status='removed' AND revision=? AND auth_epoch=? FROM workspace.team_member WHERE team_id=? AND user_id=? FOR UPDATE`, team.Target.Revision+1, team.Target.AuthEpoch+1, team.ID, p.Intent.AccountID).Scan(&removed).Error; err != nil {
					return err
				}
				if !removed {
					return writefence.ErrConflict
				}
			}
			out.Teams = append(out.Teams, team.ID)
		}
		return finishGovernanceReceipt(tx, current, out, "ready")
	})
	return out, err
}

func governanceTeams(tx *gorm.DB, account uuid.UUID) ([]workspace.GovernanceTeam, error) {
	var rows []struct {
		ID, ScopeID         uuid.UUID
		Status              string
		GovernanceBlocked   bool
		Revision, AuthEpoch int64
	}
	if err := tx.Raw(`SELECT t.id,s.id AS scope_id,t.status,t.governance_blocked,t.revision,t.auth_epoch FROM workspace.team t JOIN workspace.team_member m ON m.team_id=t.id JOIN workspace.creation_scope s ON s.team_id=t.id AND s.kind='team' WHERE m.user_id=? AND m.status<>'removed' ORDER BY t.id FOR UPDATE OF t,m,s`, account).Scan(&rows).Error; err != nil {
		return nil, err
	}
	teams := make([]workspace.GovernanceTeam, 0, len(rows))
	for _, row := range rows {
		teams = append(teams, workspace.GovernanceTeam{ID: row.ID, ScopeID: row.ScopeID, Status: row.Status, GovernanceBlocked: row.GovernanceBlocked, Revision: row.Revision, AuthEpoch: row.AuthEpoch})
	}
	for i := range teams {
		var members []workspace.GovernanceMember
		if err := tx.Raw(`SELECT user_id AS id,role,status,revision,auth_epoch FROM workspace.team_member WHERE team_id=? ORDER BY user_id FOR UPDATE`, teams[i].ID).Scan(&members).Error; err != nil {
			return nil, err
		}
		teams[i].Candidates = []workspace.GovernanceMember{}
		for _, member := range members {
			if member.ID == account {
				teams[i].Target = member
			} else if member.Role == "team_admin" && member.Status == "active" {
				teams[i].Candidates = append(teams[i].Candidates, member)
			}
		}
		if teams[i].Target.ID != account {
			return nil, writefence.ErrConflict
		}
	}
	return teams, nil
}
func lockTeamGovernanceManagement(tx *gorm.DB) error {
	return tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended('creation.team_management',0))`).Error
}

func requireNoTeamGovernancePending(tx *gorm.DB, team, except uuid.UUID) error {
	var pending bool
	if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM workspace.account_governance_preparation p CROSS JOIN LATERAL jsonb_array_elements(p.snapshot->'Teams') t WHERE p.verdict='pending' AND p.request_id<>? AND t->>'ID'=?)`, except, team.String()).Scan(&pending).Error; err != nil {
		return err
	}
	if pending {
		return writefence.ErrUnavailable
	}
	return nil
}
func lockGovernanceBlockedFence(tx *gorm.DB, control writefence.Control) error {
	if control.Validate() != nil || !control.Blocked || control.Issuer != writefence.Identity || control.Kind != writefence.Account {
		return writefence.ErrInvalid
	}
	var row struct {
		Epoch          int64
		Blocked        bool
		CoordinationID uuid.UUID
	}
	read := tx.Raw(`SELECT epoch,blocked,coordination_id FROM infra.writer_fence WHERE service_owner=infra.runtime_owner() AND issuer='identity' AND authority_kind='account' AND authority_id=? FOR UPDATE`, control.ID).Scan(&row)
	if read.Error != nil {
		return read.Error
	}
	if read.RowsAffected != 1 || row.Epoch != control.Epoch || !row.Blocked || row.CoordinationID != control.CoordinationID {
		return writefence.ErrFenced
	}
	return nil
}
func governanceTeamDigest(intent workspace.AccountGovernanceIntent, team workspace.GovernanceTeam) string {
	team.SnapshotSHA256 = ""
	encoded, _ := json.Marshal(struct {
		Intent workspace.AccountGovernanceIntent
		Team   workspace.GovernanceTeam
	}{intent, team})
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
func sameGovernanceTeams(intent workspace.AccountGovernanceIntent, expected, actual []workspace.GovernanceTeam) bool {
	if len(expected) != len(actual) {
		return false
	}
	for i := range expected {
		if expected[i].ID != actual[i].ID || expected[i].SnapshotSHA256 != governanceTeamDigest(intent, actual[i]) {
			return false
		}
	}
	return true
}
func governanceCandidateBound(p workspace.AccountGovernancePreparation, proof workspace.GovernanceQualification) bool {
	for _, team := range p.Teams {
		if team.ID == proof.TeamID && team.SnapshotSHA256 == proof.SnapshotSHA256 {
			for _, candidate := range team.Candidates {
				if candidate.ID == proof.CandidateID {
					return true
				}
			}
		}
	}
	return false
}
func governanceReceiptMatches(r workspace.AccountGovernanceReceipt, intent workspace.AccountGovernanceIntent) bool {
	return r.RequestID == intent.RequestID && r.AccountID == intent.AccountID && r.Action == intent.Action && r.InputSHA256 == intent.InputSHA256 && r.IdentityClose == intent.CloseControl
}
func finishGovernanceReceipt(tx *gorm.DB, p workspace.AccountGovernancePreparation, receipt workspace.AccountGovernanceReceipt, verdict string) error {
	if !governanceReceiptMatches(receipt, p.Intent) {
		return writefence.ErrConflict
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	result := tx.Exec(`UPDATE workspace.account_governance_preparation SET verdict=?,receipt=?::jsonb,completed_at=GREATEST(statement_timestamp(),accepted_at,COALESCE(effects_applied_at,accepted_at)) WHERE request_id=? AND verdict='pending'`, verdict, string(encoded), p.Intent.RequestID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return writefence.ErrConflict
	}
	return nil
}

var _ workspace.AccountGovernanceStore = (*AccountGovernanceStore)(nil)
