package execution

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"execution-engine/internal/workspace"

	"github.com/google/uuid"
)

const (
	nodeImage    = "node:22-alpine"
	containerDir = "/app"
	serverPort   = "3000"
	testTimeout  = 2 * time.Minute
)

type Runner struct {
	workspaces workspace.Manager
	dockerPath string

	mu         sync.Mutex
	containers map[string]string
}

func NewRunner(workspaces workspace.Manager) *Runner {
	return &Runner{workspaces: workspaces, dockerPath: "docker", containers: make(map[string]string)}
}

type TestResult struct {
	AttemptID string        `json:"attemptId"`
	Stdout    string        `json:"stdout"`
	Stderr    string        `json:"stderr"`
	ExitCode  int           `json:"exitCode"`
	Duration  time.Duration `json:"duration"`
	TimedOut  bool          `json:"timedOut"`
}

// RunTests executes npm test in an isolated Node container. Test failures are
// returned in TestResult; errors indicate that Docker could not run the command.
func (r *Runner) RunTests(attemptID string) (*TestResult, error) {
	// 1. Resolve and validate the attempt's workspace path.
	path, err := r.workspaces.GetAttemptPath(attemptID)
	if err != nil {
		return nil, err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	// 2. GetAttemptPath verifies that the workspace exists.
	// 3. Create a context with an execution timeout.
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	// 4. Record the start time.
	started := time.Now()
	containerName := "attempt-test-" + uuid.NewString()
	// 5. Mount the workspace in the Node.js container and execute npm test.
	cmd := exec.CommandContext(ctx, r.dockerPath,
		"run", "--rm", "--name", containerName,
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"--env", "NPM_CONFIG_CACHE=/tmp/npm-cache",
		"--mount", fmt.Sprintf("type=bind,source=%s,target=%s", path, containerDir),
		"--workdir", containerDir,
		nodeImage, "sh", "-lc", "npm ci && npm test",
	)
	// 6. Capture stdout and stderr.
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	result := &TestResult{
		AttemptID: attemptID,
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		ExitCode:  0,
		Duration:  time.Since(started),
	}
	// 7. Preserve the process exit code, including non-zero test failures.
	// 8. Duration is measured from command start through completion.
	if ctx.Err() == context.DeadlineExceeded {
		// CommandContext kills the Docker CLI, not necessarily the daemon-side container.
		_ = exec.Command(r.dockerPath, "rm", "--force", containerName).Run()
		result.TimedOut = true
		result.ExitCode = -1
		return result, nil
	}
	if err == nil {
		return result, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	return nil, fmt.Errorf("run tests with Docker: %w", err)
}

// StartServer starts npm start and returns its Docker container ID.
func (r *Runner) StartServer(attemptID string) (string, error) {
	// 1. Resolve and validate the attempt workspace.
	path, err := r.workspaces.GetAttemptPath(attemptID)
	if err != nil {
		return "", err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}

	// 2. Check whether a server is already running for this attempt.
	r.mu.Lock()
	defer r.mu.Unlock()
	if containerID, ok := r.containers[attemptID]; ok {
		running, err := r.containerRunning(containerID)
		if err != nil {
			return "", fmt.Errorf("check server container %s: %w", containerID, err)
		}
		if running {
			return "", fmt.Errorf("server already running for attempt %s", attemptID)
		}
		if err := exec.Command(r.dockerPath, "rm", containerID).Run(); err != nil {
			return "", fmt.Errorf("remove exited server container %s: %w", containerID, err)
		}
		delete(r.containers, attemptID)
	}
	// 3. Construct and start a detached Docker container with the workspace mounted.
	// Keep exited containers so their state can be inspected and stale IDs cleared.
	cmd := exec.Command(r.dockerPath,
		"run", "--detach",
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"--env", "NPM_CONFIG_CACHE=/tmp/npm-cache",
		"--label", "execution.attempt="+attemptID,
		"--publish", "127.0.0.1::"+serverPort,
		"--mount", fmt.Sprintf("type=bind,source=%s,target=%s", path, containerDir),
		"--workdir", containerDir,
		nodeImage, "sh", "-lc", "npm ci && npm start",
	)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("start server with Docker: %s: %w", strings.TrimSpace(string(exitErr.Stderr)), err)
		}
		return "", fmt.Errorf("start server with Docker: %w", err)
	}
	// 4–6. Capture and record the container ID for this attempt.
	containerID := strings.TrimSpace(string(out))
	if containerID == "" {
		return "", fmt.Errorf("Docker returned an empty container ID")
	}
	r.containers[attemptID] = containerID
	return containerID, nil
}

// StopServer stops and removes the container started for this attempt.
func (r *Runner) StopServer(attemptID string) error {
	// 1–2. Find the running container; return an error if none is tracked.
	r.mu.Lock()
	defer r.mu.Unlock()
	containerID, ok := r.containers[attemptID]
	if !ok {
		return fmt.Errorf("no running server for attempt %s", attemptID)
	}
	// 3–5. Force-remove the container (stops it if running), then clear the session.
	if out, err := exec.Command(r.dockerPath, "rm", "--force", containerID).CombinedOutput(); err != nil {
		return fmt.Errorf("remove server container %s: %s: %w", containerID, strings.TrimSpace(string(out)), err)
	}
	delete(r.containers, attemptID)
	return nil
}

func (r *Runner) containerRunning(containerID string) (bool, error) {
	out, err := exec.Command(r.dockerPath, "inspect", "--format", "{{.State.Running}}", containerID).Output()
	if err != nil {
		return false, err
	}
	state := strings.TrimSpace(string(out))
	if state != "true" && state != "false" {
		return false, fmt.Errorf("unexpected Docker running state %q", state)
	}
	return state == "true", nil
}
