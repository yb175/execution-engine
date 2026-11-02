package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"execution-engine/internal/execution"
	"execution-engine/internal/workspace"
)

// TestRoutesWithDocker exercises every route through HTTP and the real Docker runner.
// Enable explicitly because it runs npm ci inside Docker and requires a running daemon.
func TestRoutesWithDocker(t *testing.T) {
	if os.Getenv("RUN_DOCKER_INTEGRATION") != "1" {
		t.Skip("set RUN_DOCKER_INTEGRATION=1 to run Docker integration tests")
	}
	if out, err := exec.Command("docker", "info").CombinedOutput(); err != nil {
		t.Fatalf("Docker is not available: %v\n%s", err, out)
	}

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve integration test source path")
	}
	challengePath := filepath.Join(filepath.Dir(sourceFile), "../../challenge/broken-repo")
	if _, err := os.Stat(filepath.Join(challengePath, "package-lock.json")); err != nil {
		t.Fatalf("challenge fixture is unavailable: %v", err)
	}

	fsManager := workspace.NewFSManager(filepath.Join(t.TempDir(), "workspaces"))
	runner := execution.NewRunner(fsManager)
	server := httptest.NewServer(newRouter(fsManager, challengePath, runner))
	t.Cleanup(server.Close)

	// GET /health
	response := requestJSON(t, http.MethodGet, server.URL+"/health", "")
	assertStatus(t, response, http.StatusOK)
	var health map[string]string
	decodeBody(t, response, &health)
	if health["message"] != "Server is running" {
		t.Fatalf("health response = %#v", health)
	}

	// POST /attempts: create a real copied workspace.
	response = requestJSON(t, http.MethodPost, server.URL+"/attempts", "")
	assertStatus(t, response, http.StatusCreated)
	var created struct {
		AttemptID string `json:"attemptID"`
	}
	decodeBody(t, response, &created)
	if created.AttemptID == "" {
		t.Fatal("create response omitted attemptID")
	}
	attemptPath, err := fsManager.GetAttemptPath(created.AttemptID)
	if err != nil {
		t.Fatalf("created workspace not found: %v", err)
	}
	if _, err := os.Stat(filepath.Join(attemptPath, "package.json")); err != nil {
		t.Fatalf("challenge files were not copied into workspace: %v", err)
	}

	// Always clean up the server container if a later assertion fails.
	t.Cleanup(func() { _ = runner.StopServer(created.AttemptID) })
	payload := fmt.Sprintf(`{"attemptId":%q}`, created.AttemptID)

	// POST /run-tests: execute the actual suite in a Node container.
	response = requestJSON(t, http.MethodPost, server.URL+"/run-tests", payload)
	assertStatus(t, response, http.StatusOK)
	var testResponse struct {
		Results execution.TestResult `json:"results"`
	}
	decodeBody(t, response, &testResponse)
	if testResponse.Results.AttemptID != created.AttemptID {
		t.Fatalf("test result attemptId = %q, want %q", testResponse.Results.AttemptID, created.AttemptID)
	}
	if testResponse.Results.Duration <= 0 {
		t.Fatalf("test duration should be positive, got %s", testResponse.Results.Duration)
	}
	if testResponse.Results.TimedOut {
		t.Fatal("fixture test unexpectedly timed out")
	}

	// POST /start-server and /stop-server: exercise the real container lifecycle.
	response = requestJSON(t, http.MethodPost, server.URL+"/start-server", payload)
	assertStatus(t, response, http.StatusOK)
	var startResponse struct {
		Results string `json:"results"`
	}
	decodeBody(t, response, &startResponse)
	if startResponse.Results == "" {
		t.Fatal("start-server response omitted container ID")
	}

	response = requestJSON(t, http.MethodPost, server.URL+"/stop-server", payload)
	assertStatus(t, response, http.StatusOK)
	var stopResponse struct {
		Success   string `json:"success"`
		AttemptID string `json:"attemptId"`
	}
	decodeBody(t, response, &stopResponse)
	if stopResponse.Success != "server stopped" || stopResponse.AttemptID != created.AttemptID {
		t.Fatalf("stop-server response = %+v", stopResponse)
	}

	// DELETE /attempts: delete the workspace and verify it is gone.
	response = requestJSON(t, http.MethodDelete, server.URL+"/attempts", payload)
	assertStatus(t, response, http.StatusOK)
	response.Body.Close()
	if _, err := os.Stat(attemptPath); !os.IsNotExist(err) {
		t.Fatalf("workspace still exists after delete; stat error = %v", err)
	}
}

func requestJSON(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func assertStatus(t *testing.T, response *http.Response, want int) {
	t.Helper()
	if response.StatusCode != want {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("status = %d, want %d; body: %s", response.StatusCode, want, body)
	}
}

func decodeBody(t *testing.T, response *http.Response, dst any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(dst); err != nil {
		t.Fatalf("decode JSON response: %v", err)
	}
}
