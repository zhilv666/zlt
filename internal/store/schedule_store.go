package store

import (
	"strings"

	"zhulingtai/internal/task"
)

// Schedules live in the same SQLite database as tasks but deliberately have
// no foreign key to tasks(id): TaskStore.Save rewrites the tasks table with
// DELETE + re-insert on every save, so any FK (let alone CASCADE) would wipe
// or reject schedules as collateral. Referential integrity is enforced at the
// Runtime layer instead.
func (s *TaskStore) initScheduleSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS schedules (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			name TEXT NOT NULL,
			cron_expr TEXT NOT NULL,
			timezone TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			last_run_at TEXT NOT NULL DEFAULT '',
			last_status TEXT NOT NULL DEFAULT '',
			last_detail TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		)
	`)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`ALTER TABLE schedules ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	// One-time backfill: preserve insertion order on upgrade.
	if migrated, _ := s.metaGet("schedules_sort_order_migrated"); migrated == "" {
		if _, err = s.db.Exec(`UPDATE schedules SET sort_order = rowid WHERE sort_order = 0`); err != nil {
			return err
		}
		_ = s.metaSet("schedules_sort_order_migrated", "1")
	}
	return nil
}

func (s *TaskStore) LoadSchedules() ([]task.Schedule, error) {
	rows, err := s.db.Query(`
		SELECT id, task_id, name, cron_expr, timezone, action, enabled, last_run_at, last_status, last_detail, created_at, updated_at
		FROM schedules
		ORDER BY sort_order ASC, rowid ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []task.Schedule
	for rows.Next() {
		var sch task.Schedule
		var action string
		var enabled int

		if err := rows.Scan(
			&sch.ID,
			&sch.TaskID,
			&sch.Name,
			&sch.CronExpr,
			&sch.Timezone,
			&action,
			&enabled,
			&sch.LastRunAt,
			&sch.LastStatus,
			&sch.LastDetail,
			&sch.CreatedAt,
			&sch.UpdatedAt,
		); err != nil {
			return nil, err
		}

		sch.Action = task.ScheduleAction(action)
		sch.Enabled = enabled == 1

		schedules = append(schedules, sch)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return schedules, nil
}

func (s *TaskStore) SaveSchedules(schedules []task.Schedule) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(`DELETE FROM schedules`); err != nil {
		return err
	}

	stmt, err := tx.Prepare(`
		INSERT INTO schedules (id, task_id, name, cron_expr, timezone, action, enabled, last_run_at, last_status, last_detail, created_at, updated_at, sort_order)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, sch := range schedules {
		if _, err = stmt.Exec(
			sch.ID,
			sch.TaskID,
			sch.Name,
			sch.CronExpr,
			sch.Timezone,
			string(sch.Action),
			boolToInt(sch.Enabled),
			sch.LastRunAt,
			sch.LastStatus,
			sch.LastDetail,
			sch.CreatedAt,
			sch.UpdatedAt,
			i,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}
