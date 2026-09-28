package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/billing/application"
	"github.com/StephenQiu30/lanverse/backend/internal/billing/domain"
)

const (
	settledTopic               = "lanverse.billing.settled.v1"
	lowBalanceTopic            = "lanverse.billing.budget_low.v1"
	overrunTopic               = "lanverse.billing.budget_overrun.v1"
	settlementLedgerNotePrefix = "lanverse.billing.settlement.v1:"
)

// ErrSettlementConflict means a closed reservation does not match the
// requested actual cost, charging policy, or frozen reporting dimensions.
var ErrSettlementConflict = errors.New("operation settlement conflict")

// SettleInTransaction writes every billing fact for one terminal operation.
// tx must be the caller's active transaction; the caller writes the terminal
// operation state in that same transaction. This method does not commit or
// read/write operation tables. A replay with identical facts is read only.
func (s *Store) SettleInTransaction(ctx context.Context, tx *gorm.DB, input application.SettleInput) (application.SettleResult, error) {
	if s == nil || s.db == nil || tx == nil {
		return application.SettleResult{}, ErrUnavailable
	}
	if err := input.Validate(); err != nil {
		return application.SettleResult{}, err
	}
	input.OccurredAt = input.OccurredAt.UTC()
	tx = tx.WithContext(ctx)
	var reservation domain.Reservation
	row := tx.Raw(`
		SELECT id, project_id, operation_id, amount_micros, status, create_time, closed_at
		FROM billing.reservation
		WHERE project_id = ?::uuid AND operation_id = ?::uuid AND NOT is_delete
		FOR UPDATE
	`, input.ProjectID.String(), input.OperationID.String()).Scan(&reservation)
	if row.Error != nil {
		return application.SettleResult{}, fmt.Errorf("lock operation reservation: %w", row.Error)
	}
	if row.RowsAffected != 1 {
		return application.SettleResult{}, ErrNotFound
	}
	if err := reservation.Validate(); err != nil {
		return application.SettleResult{}, fmt.Errorf("validate operation reservation: %w", err)
	}
	if reservation.Status != domain.ReservationHeld {
		return replaySettlement(tx, input, reservation)
	}
	before := reservation
	settlement, err := reservation.Settle(input.ActualCostMicros, input.CapAtReservation, input.OccurredAt)
	if err != nil {
		return application.SettleResult{}, fmt.Errorf("calculate operation settlement: %w", err)
	}
	var budget domain.Budget
	row = tx.Raw(`
		SELECT id, project_id, limit_micros, reserved_micros, settled_micros,
		       is_overrun, revision
		FROM billing.budget
		WHERE project_id = ?::uuid AND NOT is_delete
		FOR UPDATE
	`, input.ProjectID.String()).Scan(&budget)
	if row.Error != nil {
		return application.SettleResult{}, fmt.Errorf("lock settlement budget: %w", row.Error)
	}
	if row.RowsAffected != 1 {
		return application.SettleResult{}, ErrNotFound
	}
	wasLow, err := budget.LowBalance()
	if err != nil {
		return application.SettleResult{}, fmt.Errorf("read settlement starting balance: %w", err)
	}
	wasOverrun := budget.IsOverrun
	previousRevision := budget.Revision
	if err := budget.Settle(before.AmountMicros, settlement.ChargeMicros); err != nil {
		return application.SettleResult{}, fmt.Errorf("apply budget settlement: %w", err)
	}
	availableMicros, err := budget.AvailableMicros()
	if err != nil {
		return application.SettleResult{}, fmt.Errorf("calculate settlement available balance: %w", err)
	}
	isLow, err := budget.LowBalance()
	if err != nil {
		return application.SettleResult{}, fmt.Errorf("calculate settlement low balance: %w", err)
	}
	row = tx.Exec(`
		UPDATE billing.budget
		SET reserved_micros = ?, settled_micros = ?, is_overrun = ?,
		    revision = revision + 1
		WHERE id = ?::uuid AND project_id = ?::uuid AND revision = ? AND NOT is_delete
	`, budget.ReservedMicros, budget.SettledMicros, budget.IsOverrun,
		budget.ID.String(), input.ProjectID.String(), previousRevision)
	if row.Error != nil {
		return application.SettleResult{}, fmt.Errorf("update settlement budget: %w", row.Error)
	}
	if row.RowsAffected != 1 {
		return application.SettleResult{}, domain.ErrBudgetRevision
	}
	row = tx.Exec(`
		UPDATE billing.reservation
		SET status = ?, closed_at = ?, update_time = now()
		WHERE id = ?::uuid AND project_id = ?::uuid AND operation_id = ?::uuid
		  AND status = 'held' AND NOT is_delete
	`, string(reservation.Status), input.OccurredAt, reservation.ID.String(),
		input.ProjectID.String(), input.OperationID.String())
	if row.Error != nil {
		return application.SettleResult{}, fmt.Errorf("close operation reservation: %w", row.Error)
	}
	if row.RowsAffected != 1 {
		return application.SettleResult{}, ErrSettlementConflict
	}
	result := application.SettleResult{
		ChargeMicros: settlement.ChargeMicros, ReleasedMicros: settlement.ReleasedMicros,
		ProviderOverageMicros: settlement.ProviderOverageMicros, BudgetOverrun: budget.IsOverrun,
	}
	fact, err := json.Marshal(settlementLedgerFact{
		Version:          1,
		ActualCostMicros: input.ActualCostMicros, CapAtReservation: input.CapAtReservation,
		Capability: input.Capability, BudgetOverrun: budget.IsOverrun,
		ChargeMicros: result.ChargeMicros, ReleasedMicros: result.ReleasedMicros,
		ProviderOverageMicros: result.ProviderOverageMicros,
		EpisodeID:             input.EpisodeID, ShotID: input.ShotID,
		ModelKey: input.ModelKey, Region: input.Region,
	})
	if err != nil {
		return application.SettleResult{}, fmt.Errorf("encode settlement ledger fact: %w", err)
	}
	for _, entry := range []struct {
		typeName string
		amount   int64
		write    bool
		note     string
	}{
		{"settle", settlement.ChargeMicros, true, settlementLedgerNotePrefix + string(fact)},
		{"release", settlement.ReleasedMicros, settlement.ReleasedMicros > 0, ""},
		{"provider_overage", settlement.ProviderOverageMicros, settlement.ProviderOverageMicros > 0, ""},
	} {
		if entry.write {
			if err := insertSettlementLedger(tx, input, entry.typeName, entry.amount, entry.note); err != nil {
				return application.SettleResult{}, err
			}
		}
	}
	if err := insertSettlementEvent(tx, input, result, budget, availableMicros, !wasLow && isLow, wasOverrun); err != nil {
		return application.SettleResult{}, err
	}
	return result, nil
}

type settlementLedgerFact struct {
	Version               int        `json:"version"`
	ActualCostMicros      int64      `json:"actual_cost_micros"`
	CapAtReservation      bool       `json:"cap_at_reservation"`
	Capability            string     `json:"capability"`
	BudgetOverrun         bool       `json:"budget_overrun"`
	ChargeMicros          int64      `json:"charge_micros"`
	ReleasedMicros        int64      `json:"released_micros"`
	ProviderOverageMicros int64      `json:"provider_overage_micros"`
	EpisodeID             *uuid.UUID `json:"episode_id,omitempty"`
	ShotID                *uuid.UUID `json:"shot_id,omitempty"`
	ModelKey              *string    `json:"model_key,omitempty"`
	Region                *string    `json:"region,omitempty"`
}

func insertSettlementLedger(tx *gorm.DB, input application.SettleInput, entryType string, amount int64, note string) error {
	row := tx.Exec(`
		INSERT INTO billing.ledger_entry
		  (id, project_id, entry_type, amount_micros, operation_id,
		   episode_id, shot_id, model_key, region, note, create_time)
		VALUES (?::uuid, ?::uuid, ?, ?, ?::uuid,
		        ?::uuid, ?::uuid, ?, ?, ?, ?)
	`, uuid.NewString(), input.ProjectID.String(), entryType, amount,
		input.OperationID.String(), input.EpisodeID, input.ShotID,
		input.ModelKey, input.Region, note, input.OccurredAt)
	if row.Error != nil {
		return fmt.Errorf("insert %s settlement ledger: %w", entryType, row.Error)
	}
	if row.RowsAffected != 1 {
		return fmt.Errorf("insert %s settlement ledger: wrote %d rows", entryType, row.RowsAffected)
	}
	return nil
}

type settlementEventData struct {
	OperationID      uuid.UUID  `json:"operation_id"`
	ActualCostMicros int64      `json:"actual_cost_micros"`
	CapAtReservation bool       `json:"cap_at_reservation"`
	ChargeMicros     int64      `json:"charge_micros"`
	ReleasedMicros   int64      `json:"released_micros"`
	OverageMicros    int64      `json:"overage_micros"`
	AvailableMicros  int64      `json:"available_micros"`
	BudgetOverrun    bool       `json:"budget_overrun"`
	Capability       string     `json:"capability"`
	EpisodeID        *uuid.UUID `json:"episode_id,omitempty"`
	ShotID           *uuid.UUID `json:"shot_id,omitempty"`
	ModelKey         *string    `json:"model_key,omitempty"`
	Region           *string    `json:"region,omitempty"`
}

func insertSettlementEvent(tx *gorm.DB, input application.SettleInput, result application.SettleResult, budget domain.Budget, availableMicros int64, becameLow, wasOverrun bool) error {
	var project struct{ OrgID uuid.UUID }
	row := tx.Raw(`SELECT org_id FROM workspace.project WHERE id = ?::uuid`, input.ProjectID.String()).Scan(&project)
	if row.Error != nil {
		return fmt.Errorf("read settlement organization: %w", row.Error)
	}
	if row.RowsAffected != 1 || project.OrgID == uuid.Nil {
		return ErrNotFound
	}
	data := settlementEventData{
		OperationID: input.OperationID, ActualCostMicros: input.ActualCostMicros,
		CapAtReservation: input.CapAtReservation, ChargeMicros: result.ChargeMicros,
		ReleasedMicros: result.ReleasedMicros, OverageMicros: result.ProviderOverageMicros,
		AvailableMicros: availableMicros,
		BudgetOverrun:   result.BudgetOverrun, Capability: input.Capability,
		EpisodeID: input.EpisodeID, ShotID: input.ShotID,
		ModelKey: input.ModelKey, Region: input.Region,
	}
	if err := writeBillingEvent(tx, settledTopic, project.OrgID, input.ProjectID, "operation", input.OperationID, input.OccurredAt, data, nil); err != nil {
		return err
	}
	if becameLow {
		if err := writeBillingEvent(tx, lowBalanceTopic, project.OrgID, input.ProjectID, "budget", budget.ID, input.OccurredAt, map[string]any{
			"limit_micros": budget.LimitMicros, "available_micros": availableMicros,
			"is_overrun": budget.IsOverrun,
		}, &budget.Revision); err != nil {
			return err
		}
	}
	if result.BudgetOverrun && !wasOverrun {
		return writeBillingEvent(tx, overrunTopic, project.OrgID, input.ProjectID, "budget", budget.ID, input.OccurredAt, map[string]any{
			"operation_id": input.OperationID, "budget_id": budget.ID,
			"is_overrun": true,
		}, nil)
	}
	return nil
}

func writeBillingEvent(tx *gorm.DB, topic string, orgID, projectID uuid.UUID, aggregateType string, aggregateID uuid.UUID, occurredAt time.Time, data any, revision *int64) error {
	eventID := uuid.New()
	aggregate := map[string]any{"type": aggregateType, "id": aggregateID}
	if revision != nil {
		aggregate["revision"] = *revision
	}
	payload, err := json.Marshal(map[string]any{
		"event_id": eventID, "event_type": topic, "occurred_at": occurredAt,
		"org_id": orgID, "project_id": projectID,
		"actor":     map[string]any{"kind": "system", "id": nil},
		"aggregate": aggregate,
		"data":      data,
	})
	if err != nil {
		return fmt.Errorf("encode %s event: %w", topic, err)
	}
	row := tx.Exec(`
		INSERT INTO infra.outbox (id, topic, partition_key, payload)
		VALUES (?::uuid, ?, ?, ?::jsonb)
	`, eventID.String(), topic, projectID.String(), string(payload))
	if row.Error != nil {
		return fmt.Errorf("insert %s event: %w", topic, row.Error)
	}
	if row.RowsAffected != 1 {
		return fmt.Errorf("insert %s event: wrote %d rows", topic, row.RowsAffected)
	}
	return nil
}

func replaySettlement(tx *gorm.DB, input application.SettleInput, reservation domain.Reservation) (application.SettleResult, error) {
	var ledger []struct {
		EntryType    string
		AmountMicros int64
		EpisodeID    *uuid.UUID
		ShotID       *uuid.UUID
		ModelKey     *string
		Region       *string
		Note         string
	}
	if err := tx.Raw(`
		SELECT entry_type, amount_micros, episode_id, shot_id, model_key, region, note
		FROM billing.ledger_entry
		WHERE project_id = ?::uuid AND operation_id = ?::uuid
		  AND entry_type IN ('settle', 'release', 'provider_overage') AND NOT is_delete
	`, input.ProjectID.String(), input.OperationID.String()).Scan(&ledger).Error; err != nil {
		return application.SettleResult{}, fmt.Errorf("read closed reservation ledger: %w", err)
	}
	expected := application.SettleResult{ChargeMicros: input.ActualCostMicros}
	if input.CapAtReservation && input.ActualCostMicros > reservation.AmountMicros {
		expected.ChargeMicros = reservation.AmountMicros
		expected.ProviderOverageMicros = input.ActualCostMicros - reservation.AmountMicros
	}
	if expected.ChargeMicros < reservation.AmountMicros {
		expected.ReleasedMicros = reservation.AmountMicros - expected.ChargeMicros
	}
	if (expected.ChargeMicros == 0 && reservation.Status != domain.ReservationReleased) ||
		(expected.ChargeMicros > 0 && reservation.Status != domain.ReservationSettled) {
		return application.SettleResult{}, ErrSettlementConflict
	}
	want := map[string]int64{"settle": expected.ChargeMicros}
	if expected.ReleasedMicros > 0 {
		want["release"] = expected.ReleasedMicros
	}
	if expected.ProviderOverageMicros > 0 {
		want["provider_overage"] = expected.ProviderOverageMicros
	}
	if len(ledger) != len(want) {
		return application.SettleResult{}, ErrSettlementConflict
	}
	for _, entry := range ledger {
		amount, present := want[entry.EntryType]
		if !present || amount != entry.AmountMicros ||
			!equalUUID(entry.EpisodeID, input.EpisodeID) || !equalUUID(entry.ShotID, input.ShotID) ||
			!equalString(entry.ModelKey, input.ModelKey) || !equalString(entry.Region, input.Region) {
			return application.SettleResult{}, ErrSettlementConflict
		}
		if entry.EntryType == "settle" {
			var fact settlementLedgerFact
			if err := decodeSettlementLedgerFact(entry.Note, &fact); err != nil ||
				fact.Version != 1 ||
				fact.ActualCostMicros != input.ActualCostMicros ||
				fact.CapAtReservation != input.CapAtReservation || fact.Capability != input.Capability ||
				fact.ChargeMicros != expected.ChargeMicros ||
				fact.ReleasedMicros != expected.ReleasedMicros ||
				fact.ProviderOverageMicros != expected.ProviderOverageMicros ||
				!equalUUID(fact.EpisodeID, input.EpisodeID) || !equalUUID(fact.ShotID, input.ShotID) ||
				!equalString(fact.ModelKey, input.ModelKey) || !equalString(fact.Region, input.Region) {
				return application.SettleResult{}, ErrSettlementConflict
			}
			expected.BudgetOverrun = fact.BudgetOverrun
		} else if entry.Note != "" {
			return application.SettleResult{}, ErrSettlementConflict
		}
		delete(want, entry.EntryType)
	}
	if len(want) != 0 {
		return application.SettleResult{}, ErrSettlementConflict
	}
	expected.AlreadySettled = true
	return expected, nil
}

func decodeSettlementLedgerFact(note string, fact *settlementLedgerFact) error {
	if !strings.HasPrefix(note, settlementLedgerNotePrefix) || len(note) > 2048 {
		return ErrSettlementConflict
	}
	body := []byte(strings.TrimPrefix(note, settlementLedgerNotePrefix))
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return err
	}
	for _, key := range []string{
		"version", "actual_cost_micros", "cap_at_reservation", "capability",
		"budget_overrun", "charge_micros", "released_micros", "provider_overage_micros",
	} {
		if _, present := fields[key]; !present {
			return ErrSettlementConflict
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(fact); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return ErrSettlementConflict
	}
	return nil
}

func equalUUID(a, b *uuid.UUID) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func equalString(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
