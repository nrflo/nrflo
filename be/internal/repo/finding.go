package repo

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"be/internal/clock"
	"be/internal/db"
)

// Actor identifies the principal that made a findings mutation.
type Actor struct {
	ID     string // session_id, user_id, token_id, or empty for system
	Source string // "agent", "orchestrator", "user", "service_token", "system"
}

// Denorm carries denormalized columns stored on each findings row.
type Denorm struct {
	ProjectID          string
	WorkflowInstanceID string
	AgentType          string
	ModelID            string
}

// FindingRepo handles findings CRUD with per-mutation history tracking.
type FindingRepo struct {
	db    db.Querier
	clock clock.Clock
}

// NewFindingRepo creates a new FindingRepo.
func NewFindingRepo(database db.Querier, clk clock.Clock) *FindingRepo {
	return &FindingRepo{db: database, clock: clk}
}

func findingNullStr(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// Upsert inserts or updates a single finding and records an 'add' history row.
func (r *FindingRepo) Upsert(scope, scopeID, key string, value json.RawMessage, denorm Denorm, actor Actor) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	now := r.clock.Now().UTC().Format(time.RFC3339Nano)
	valStr := string(value)

	var existingID string
	var oldValue sql.NullString
	existErr := tx.QueryRow(
		`SELECT id, value FROM findings WHERE scope=? AND scope_id=? AND key=?`,
		scope, scopeID, key,
	).Scan(&existingID, &oldValue)

	var findingID string
	if existErr == sql.ErrNoRows {
		findingID = uuid.New().String()
		if _, err = tx.Exec(
			`INSERT INTO findings
			 (id, scope, scope_id, key, value, project_id, workflow_instance_id, agent_type, model_id,
			  created_at, created_by, created_source, updated_at, updated_by, updated_source, write_count)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
			findingID, scope, scopeID, key, valStr,
			findingNullStr(denorm.ProjectID), findingNullStr(denorm.WorkflowInstanceID),
			findingNullStr(denorm.AgentType), findingNullStr(denorm.ModelID),
			now, findingNullStr(actor.ID), actor.Source,
			now, findingNullStr(actor.ID), actor.Source,
		); err != nil {
			return err
		}
	} else if existErr != nil {
		return existErr
	} else {
		findingID = existingID
		if _, err = tx.Exec(
			`UPDATE findings SET value=?, updated_at=?, updated_by=?, updated_source=?, write_count=write_count+1 WHERE id=?`,
			valStr, now, findingNullStr(actor.ID), actor.Source, findingID,
		); err != nil {
			return err
		}
	}

	if err := writeHistoryTx(tx, HistoryRow{
		ID:          uuid.New().String(),
		FindingID:   sql.NullString{String: findingID, Valid: true},
		Scope:       scope, ScopeID: scopeID, Key: key,
		Operation: "add",
		OldValue:  oldValue,
		NewValue:  sql.NullString{String: valStr, Valid: true},
		ActorID:   actor.ID, ActorSource: actor.Source,
		CreatedAt: r.clock.Now().UTC(),
	}); err != nil {
		return err
	}

	return tx.Commit()
}

// Append merges newValue into the existing finding using array-merge semantics,
// recording an 'append' history row.
func (r *FindingRepo) Append(scope, scopeID, key string, newValue json.RawMessage, denorm Denorm, actor Actor) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	now := r.clock.Now().UTC().Format(time.RFC3339Nano)

	var existingID string
	var oldValueStr sql.NullString
	existErr := tx.QueryRow(
		`SELECT id, value FROM findings WHERE scope=? AND scope_id=? AND key=?`,
		scope, scopeID, key,
	).Scan(&existingID, &oldValueStr)

	var merged json.RawMessage
	var findingID string

	var parsedNew interface{}
	json.Unmarshal(newValue, &parsedNew) //nolint:errcheck

	if existErr == sql.ErrNoRows {
		merged = newValue
		findingID = uuid.New().String()
	} else if existErr != nil {
		return existErr
	} else {
		findingID = existingID
		var parsedOld interface{}
		json.Unmarshal([]byte(oldValueStr.String), &parsedOld) //nolint:errcheck
		mergedIface := findingAppendValue(parsedOld, parsedNew)
		b, _ := json.Marshal(mergedIface)
		merged = json.RawMessage(b)
	}

	mergedStr := string(merged)

	if existErr == sql.ErrNoRows {
		if _, err = tx.Exec(
			`INSERT INTO findings
			 (id, scope, scope_id, key, value, project_id, workflow_instance_id, agent_type, model_id,
			  created_at, created_by, created_source, updated_at, updated_by, updated_source, write_count)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
			findingID, scope, scopeID, key, mergedStr,
			findingNullStr(denorm.ProjectID), findingNullStr(denorm.WorkflowInstanceID),
			findingNullStr(denorm.AgentType), findingNullStr(denorm.ModelID),
			now, findingNullStr(actor.ID), actor.Source,
			now, findingNullStr(actor.ID), actor.Source,
		); err != nil {
			return err
		}
	} else {
		if _, err = tx.Exec(
			`UPDATE findings SET value=?, updated_at=?, updated_by=?, updated_source=?, write_count=write_count+1 WHERE id=?`,
			mergedStr, now, findingNullStr(actor.ID), actor.Source, findingID,
		); err != nil {
			return err
		}
	}

	if err := writeHistoryTx(tx, HistoryRow{
		ID:          uuid.New().String(),
		FindingID:   sql.NullString{String: findingID, Valid: true},
		Scope:       scope, ScopeID: scopeID, Key: key,
		Operation: "append",
		OldValue:  oldValueStr,
		NewValue:  sql.NullString{String: mergedStr, Valid: true},
		ActorID:   actor.ID, ActorSource: actor.Source,
		CreatedAt: r.clock.Now().UTC(),
	}); err != nil {
		return err
	}

	return tx.Commit()
}

// DeleteKeys deletes specific keys and records 'delete' history rows.
// Returns the keys that were actually deleted (existing keys only).
func (r *FindingRepo) DeleteKeys(scope, scopeID string, keys []string, actor Actor) ([]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}

	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	var deleted []string

	for _, key := range keys {
		var findingID string
		var oldValue sql.NullString
		err := tx.QueryRow(
			`SELECT id, value FROM findings WHERE scope=? AND scope_id=? AND key=?`,
			scope, scopeID, key,
		).Scan(&findingID, &oldValue)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, err
		}

		if _, err = tx.Exec(`DELETE FROM findings WHERE id=?`, findingID); err != nil {
			return nil, err
		}

		if err := writeHistoryTx(tx, HistoryRow{
			ID:          uuid.New().String(),
			FindingID:   sql.NullString{}, // finding deleted; FK becomes NULL via ON DELETE SET NULL
			Scope:       scope, ScopeID: scopeID, Key: key,
			Operation: "delete",
			OldValue:  oldValue,
			NewValue:  sql.NullString{},
			ActorID:   actor.ID, ActorSource: actor.Source,
			CreatedAt: r.clock.Now().UTC(),
		}); err != nil {
			return nil, err
		}

		// Stamp history row with the finding_id before delete (FK ON DELETE SET NULL fires after)
		tx.Exec( //nolint:errcheck
			`UPDATE findings_history SET finding_id=? WHERE id=(SELECT id FROM findings_history ORDER BY rowid DESC LIMIT 1)`,
			findingID,
		)
		deleted = append(deleted, key)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return deleted, nil
}

