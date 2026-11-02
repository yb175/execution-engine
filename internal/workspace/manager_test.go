package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateGetDeleteAttempt(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "app.js"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "node_modules", "pkg"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "node_modules", "pkg", "index.js"), []byte("dependency"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "src", "node_modules"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "src", "node_modules", "nested.js"), []byte("dependency"), 0644); err != nil {
		t.Fatal(err)
	}

	manager := NewFSManager(filepath.Join(root, "workspaces"))
	if err := manager.CreateAttempt("attempt-1", source); err != nil {
		t.Fatal(err)
	}
	path, err := manager.GetAttemptPath("attempt-1")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(path, "app.js"))
	if err != nil || string(got) != "test" {
		t.Fatalf("copied file = %q, err = %v", got, err)
	}
	for _, dependencyPath := range []string{
		filepath.Join(path, "node_modules"),
		filepath.Join(path, "src", "node_modules"),
	} {
		if _, err := os.Stat(dependencyPath); !os.IsNotExist(err) {
			t.Errorf("node_modules should not be copied at %s; stat error = %v", dependencyPath, err)
		}
	}
	if err := manager.DeleteAttempt("attempt-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.GetAttemptPath("attempt-1"); !os.IsNotExist(err) {
		t.Fatalf("expected missing attempt, got %v", err)
	}
}

func TestAttemptIDCannotEscapeWorkspace(t *testing.T) {
	manager := NewFSManager(t.TempDir())
	if err := manager.DeleteAttempt("../outside"); err == nil {
		t.Fatal("expected invalid attempt ID error")
	}
}
