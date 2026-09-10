package store

import "fmt"

// ReorderTasks updates only the sort_order column for the given task IDs, in
// the order they appear in ids. It does not touch any other column and does
// not DELETE/re-insert, so running processes, cron registrations and session
// state are unaffected. The caller (Runtime) validates that ids is a complete
// permutation of the current task set before calling.
func (s *TaskStore) ReorderTasks(ids []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`UPDATE tasks SET sort_order = ? WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, id := range ids {
		res, err := stmt.Exec(i, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("task %q not found during reorder", id)
		}
	}

	return tx.Commit()
}

// ReorderSchedules is the schedule-table counterpart of ReorderTasks.
func (s *TaskStore) ReorderSchedules(ids []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`UPDATE schedules SET sort_order = ? WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, id := range ids {
		res, err := stmt.Exec(i, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("schedule %q not found during reorder", id)
		}
	}

	return tx.Commit()
}
