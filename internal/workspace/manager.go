package workspace

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type Manager interface {
	CreateAttempt(attemptID string, challengePath string) error
	DeleteAttempt(attemptID string) error
	GetAttemptPath(attemptID string) (string, error)
}

type FsManager struct{ path string }

func NewFSManager(path string) *FsManager { return &FsManager{path: path} }

func (m *FsManager) attemptPath(id string) (string, error) {
	if id == "" || filepath.Base(id) != id || id == "." || id == ".." {
		return "", fmt.Errorf("invalid attempt id %q", id)
	}
	return filepath.Join(m.path, id), nil
}

func (m *FsManager) CreateAttempt(attemptID, challengePath string) error {
	dest, err := m.attemptPath(attemptID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(m.path, 0755); err != nil {
		return err
	}
	if err := os.Mkdir(dest, 0755); err != nil {
		return err
	}
	if err := copyChallenge(challengePath, dest); err != nil {
		_ = os.RemoveAll(dest)
		return err
	}
	return nil
}

func copyChallenge(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == source {
			return nil
		}
		if entry.Name() == "node_modules" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported challenge file type: %s", path)
		}

		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
		if err != nil {
			input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		inputErr := input.Close()
		outputErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if inputErr != nil {
			return inputErr
		}
		return outputErr
	})
}

func (m *FsManager) GetAttemptPath(attemptID string) (string, error) {
	path, err := m.attemptPath(attemptID)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("attempt path is not a directory")
	}
	return path, nil
}

func (m *FsManager) DeleteAttempt(attemptID string) error {
	path, err := m.GetAttemptPath(attemptID)
	if err != nil {
		return err
	}
	return os.RemoveAll(path)
}
