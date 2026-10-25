package postgres

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ReadUserCommand is serialized by the existing platform-settings transaction lock.
func (t *governanceTx) ReadUserCommand(key uuid.UUID) (application.UserCommandReceipt, bool, error) {
	var row struct {
		RequestHash  string
		ResponseBody []byte
	}
	r := t.db.Raw(`SELECT request_hash,response_body FROM infra.idempotency_record WHERE actor_id=? AND idem_key=? AND NOT is_delete`, t.actor.ID, "account.command:"+key.String()).Scan(&row)
	var out application.UserCommandReceipt
	if r.Error != nil || r.RowsAffected == 0 {
		return out, false, r.Error
	}
	if err := json.Unmarshal(row.ResponseBody, &out); err != nil {
		return out, false, err
	}
	out.RequestHash = row.RequestHash
	return out, true, nil
}

// SaveUserCommand commits a receipt atomically with the mutation and governance event.
func (t *governanceTx) SaveUserCommand(receipt application.UserCommandReceipt, now time.Time) error {
	body, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	return t.db.Exec(`INSERT INTO infra.idempotency_record(id,actor_id,idem_key,request_hash,status_code,response_body,expires_at,create_time,update_time) VALUES(?,?,?,?,204,?::jsonb,?,?,?)`, uuid.New(), t.actor.ID, "account.command:"+receipt.RequestID.String(), receipt.RequestHash, string(body), now.Add(24*time.Hour), now, now).Error
}
