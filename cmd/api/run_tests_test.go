package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"execution-engine/internal/execution"
	"execution-engine/internal/workspace"
)

type stubTestRunner struct {
	result    *execution.TestResult
	err       error
	attemptID string
	calls     int
}

func (s *stubTestRunner) RunTests(attemptID string) (*execution.TestResult, error) {
	s.calls++
	s.attemptID = attemptID
	return s.result, s.err
}

func (s *stubTestRunner) StartServer(string) (string, error) {
	return "", errors.New("not implemented in test stub")
}

func (s *stubTestRunner) StopServer(string) error {
	return errors.New("not implemented in test stub")
}

func TestRunTestsEndpointReturnsExecutionResult(t *testing.T) {
	fsManager := workspace.NewFSManager(t.TempDir())
	runner := &stubTestRunner{result: &execution.TestResult{
		AttemptID: "attempt-123",
		Stdout:    "2 tests passed",
		Stderr:    "warning",
		ExitCode:  1,
		Duration:  125 * time.Millisecond,
		TimedOut:  false,
	}}
	server := httptest.NewServer(newRouter(fsManager, t.TempDir(), runner))
	t.Cleanup(server.Close)

	response, err := http.Post(server.URL+"/run-tests", "application/json", strings.NewReader(`{"attemptId":"attempt-123"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if !strings.Contains(response.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", response.Header.Get("Content-Type"))
	}

	var body struct {
		Results execution.TestResult `json:"results"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Results.AttemptID != "attempt-123" || body.Results.Stdout != "2 tests passed" || body.Results.Stderr != "warning" || body.Results.ExitCode != 1 || body.Results.TimedOut {
		t.Errorf("unexpected test result: %+v", body.Results)
	}
	if body.Results.Duration != 125*time.Millisecond {
		t.Errorf("duration = %s, want %s", body.Results.Duration, 125*time.Millisecond)
	}
	if runner.calls != 1 || runner.attemptID != "attempt-123" {
		t.Errorf("runner called %d times with attempt ID %q", runner.calls, runner.attemptID)
	}
}

func TestRunTestsEndpointRejectsInvalidJSON(t *testing.T) {
	runner := &stubTestRunner{}
	server := httptest.NewServer(newRouter(workspace.NewFSManager(t.TempDir()), t.TempDir(), runner))
	t.Cleanup(server.Close)

	response, err := http.Post(server.URL+"/run-tests", "application/json", strings.NewReader(`{"attemptId":`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusBadRequest)
	}
	if runner.calls != 0 {
		t.Fatalf("runner called %d times for invalid request", runner.calls)
	}
}

func TestRunTestsEndpointRejectsMissingAttemptID(t *testing.T) {
	runner := &stubTestRunner{}
	server := httptest.NewServer(newRouter(workspace.NewFSManager(t.TempDir()), t.TempDir(), runner))
	t.Cleanup(server.Close)

	response, err := http.Post(server.URL+"/run-tests", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusBadRequest)
	}
	if runner.calls != 0 {
		t.Fatalf("runner called %d times without attempt ID", runner.calls)
	}
}

func TestRunTestsEndpointMapsRunnerErrors(t *testing.T) {
	cases := []struct {
		name string
		id   string
		err  error
		want int
	}{
		{name: "attempt workspace not found", id: "attempt-123", err: os.ErrNotExist, want: http.StatusNotFound},
		{name: "invalid attempt ID", id: "../outside", err: errors.New("invalid attempt ID"), want: http.StatusBadRequest},
		{name: "execution infrastructure failure", id: "attempt-123", err: errors.New("docker unavailable"), want: http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &stubTestRunner{err: tc.err}
			server := httptest.NewServer(newRouter(workspace.NewFSManager(t.TempDir()), t.TempDir(), runner))
			t.Cleanup(server.Close)

			response, err := http.Post(server.URL+"/run-tests", "application/json", strings.NewReader(`{"attemptId":"`+tc.id+`"}`))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", response.StatusCode, tc.want)
			}
			var body map[string]string
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["error"] == "" {
				t.Fatal("error response omitted error message")
			}
		})
	}
}
