package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type StepStatus string

const (
	StatusDone    StepStatus = "done"
	StatusFailed  StepStatus = "failed"
	StatusRunning StepStatus = "running"
)

type StepRecord struct {
	Status    StepStatus `json:"status"`
	Message   string     `json:"message,omitempty"`
	UpdatedAt string     `json:"updatedAt"`
}

type File struct {
	Version    int                   `json:"version"`
	Cluster    string                `json:"cluster"`
	ConfigPath string                `json:"configPath"`
	UpdatedAt  string                `json:"updatedAt"`
	Steps      map[string]StepRecord `json:"steps"`
}

type Store struct {
	Path string
	Data File
}

func DefaultPath(clusterName string) string {
	base, err := os.UserHomeDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, ".liteboxd-installer", fmt.Sprintf("%s-state.json", clusterName))
}

func LoadOrCreate(path, clusterName, configPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}

	s := &Store{Path: path}
	if _, err := os.Stat(path); err == nil {
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("read state file: %w", readErr)
		}
		if unmarshalErr := json.Unmarshal(b, &s.Data); unmarshalErr != nil {
			return nil, fmt.Errorf("parse state file: %w", unmarshalErr)
		}
		if s.Data.Steps == nil {
			s.Data.Steps = map[string]StepRecord{}
		}
		return s, nil
	}

	s.Data = File{
		Version:    1,
		Cluster:    clusterName,
		ConfigPath: configPath,
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
		Steps:      map[string]StepRecord{},
	}
	if err := s.Save(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Mark(step string, status StepStatus, message string) error {
	s.Data.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.Data.Steps[step] = StepRecord{
		Status:    status,
		Message:   message,
		UpdatedAt: s.Data.UpdatedAt,
	}
	return s.Save()
}

func (s *Store) Save() error {
	b, err := json.MarshalIndent(s.Data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err := os.WriteFile(s.Path, b, 0o644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}
