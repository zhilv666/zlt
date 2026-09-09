package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"

	"zhulingtai/internal/task"
)

type TaskStore struct {
	db       *sql.DB
	dbPath   string
	jsonPath string
}

func NewTaskStore(dbPath string, jsonPath string) (*TaskStore, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	store := &TaskStore{
		db:       db,
		dbPath:   dbPath,
		jsonPath: jsonPath,
	}

	if err := store.initMetaTable(); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := store.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := store.initScheduleSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := store.initSettingsSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := store.bootstrapFromJSONIfNeeded(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *TaskStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *TaskStore) Load() ([]task.Config, error) {
	rows, err := s.db.Query(`
		SELECT id, name, program, args_json, workdir, env_json, autostart, restart_on_crash, stop_timeout_sec, restart_delay_sec, max_restart_count, health_check_url, health_check_interval_sec, health_check_failure_threshold
		FROM tasks
		ORDER BY sort_order ASC, rowid ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []task.Config
	for rows.Next() {
		var cfg task.Config
		var argsJSON string
		var envJSON string
		var autostart int
		var restartOnCrash int

		if err := rows.Scan(
			&cfg.ID,
			&cfg.Name,
			&cfg.Program,
			&argsJSON,
			&cfg.WorkDir,
			&envJSON,
			&autostart,
			&restartOnCrash,
			&cfg.StopTimeoutSec,
			&cfg.RestartDelaySec,
			&cfg.MaxRestartCount,
			&cfg.HealthCheckURL,
			&cfg.HealthCheckIntervalSec,
			&cfg.HealthCheckFailureThreshold,
		); err != nil {
			return nil, err
		}

		cfg.AutoStart = autostart == 1
		cfg.RestartOnCrash = restartOnCrash == 1

		if err := json.Unmarshal([]byte(argsJSON), &cfg.Args); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(envJSON), &cfg.Env); err != nil {
			return nil, err
		}

		tasks = append(tasks, cfg)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (s *TaskStore) Save(tasks []task.Config) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	// 统一延迟回滚：提交成功后 Rollback 返回 ErrTxDone（已忽略）；
	// 任一错误路径都会回滚未提交事务。此前循环内用 := 重新声明 err
	// 遮蔽外层，导致循环中 marshal/exec 失败时 defer 看到的外层 err 仍为
	// nil 而跳过回滚，事务被遗弃并泄漏连接。循环内改用独立变量 mErr
	// 彻底消除遮蔽，并由无条件延迟回滚兜底。
	defer func() { _ = tx.Rollback() }()

	if _, err = tx.Exec(`DELETE FROM tasks`); err != nil {
		return err
	}

	stmt, err := tx.Prepare(`
		INSERT INTO tasks (id, name, program, args_json, workdir, env_json, autostart, restart_on_crash, stop_timeout_sec, restart_delay_sec, max_restart_count, health_check_url, health_check_interval_sec, health_check_failure_threshold, sort_order)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, cfg := range tasks {
		argsJSON, mErr := json.Marshal(cfg.Args)
		if mErr != nil {
			return mErr
		}
		envJSON, mErr := json.Marshal(cfg.Env)
		if mErr != nil {
			return mErr
		}

		if _, err = stmt.Exec(
			cfg.ID,
			cfg.Name,
			cfg.Program,
			string(argsJSON),
			cfg.WorkDir,
			string(envJSON),
			boolToInt(cfg.AutoStart),
			boolToInt(cfg.RestartOnCrash),
			cfg.StopTimeoutSec,
			cfg.RestartDelaySec,
			cfg.MaxRestartCount,
			cfg.HealthCheckURL,
			cfg.HealthCheckIntervalSec,
			cfg.HealthCheckFailureThreshold,
			i,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *TaskStore) initSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			program TEXT NOT NULL,
			args_json TEXT NOT NULL,
			workdir TEXT NOT NULL,
			env_json TEXT NOT NULL,
			autostart INTEGER NOT NULL DEFAULT 0,
			restart_on_crash INTEGER NOT NULL DEFAULT 0,
			stop_timeout_sec INTEGER NOT NULL DEFAULT 8,
			restart_delay_sec INTEGER NOT NULL DEFAULT 2,
			max_restart_count INTEGER NOT NULL DEFAULT 0,
			health_check_url TEXT NOT NULL DEFAULT '',
			health_check_interval_sec INTEGER NOT NULL DEFAULT 0,
			health_check_failure_threshold INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`ALTER TABLE tasks ADD COLUMN restart_delay_sec INTEGER NOT NULL DEFAULT 2`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	_, err = s.db.Exec(`ALTER TABLE tasks ADD COLUMN max_restart_count INTEGER NOT NULL DEFAULT 0`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	_, err = s.db.Exec(`ALTER TABLE tasks ADD COLUMN health_check_url TEXT NOT NULL DEFAULT ''`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	_, err = s.db.Exec(`ALTER TABLE tasks ADD COLUMN health_check_interval_sec INTEGER NOT NULL DEFAULT 0`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	_, err = s.db.Exec(`ALTER TABLE tasks ADD COLUMN health_check_failure_threshold INTEGER NOT NULL DEFAULT 0`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	_, err = s.db.Exec(`ALTER TABLE tasks ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	// One-time backfill: copy rowid into sort_order so existing rows keep their
	// insertion order. A meta marker prevents re-backfilling after the first
	// migration — subsequent Save() calls write sort_order = slice index.
	if migrated, _ := s.metaGet("tasks_sort_order_migrated"); migrated == "" {
		if _, err = s.db.Exec(`UPDATE tasks SET sort_order = rowid WHERE sort_order = 0`); err != nil {
			return err
		}
		_ = s.metaSet("tasks_sort_order_migrated", "1")
	}
	return nil
}

func (s *TaskStore) bootstrapFromJSONIfNeeded() error {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM tasks`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	tasks, err := s.loadLegacyJSON()
	if err != nil {
		return err
	}
	return s.Save(tasks)
}

func (s *TaskStore) loadLegacyJSON() ([]task.Config, error) {
	data, err := os.ReadFile(s.jsonPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []task.Config{}, nil
		}
		return nil, err
	}

	var tasks []task.Config
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
