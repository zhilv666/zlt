package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"tray/internal/task"
)

type TaskStore struct {
	path string
}

func NewTaskStore(path string) *TaskStore {
	return &TaskStore{path: path}
}

func (s *TaskStore) Load() ([]task.Config, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []task.Config{task.DefaultOpenListTask()}, nil
		}
		return nil, err
	}

	var tasks []task.Config
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		tasks = []task.Config{task.DefaultOpenListTask()}
	}
	return tasks, nil
}

func (s *TaskStore) Save(tasks []task.Config) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0o644)
}
