package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"execution-engine/internal/execution"
	"execution-engine/internal/workspace"
)

func testRouter(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "challenge")
	if err := os.Mkdir(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "sample.txt"), []byte("challenge"), 0644); err != nil {
		t.Fatal(err)
	}
	workspaces := filepath.Join(root, "workspaces")
	fsManager := workspace.NewFSManager(workspaces)
	server := httptest.NewServer(newRouter(fsManager, source, execution.NewRunner(fsManager)))
	t.Cleanup(server.Close)
	return server, workspaces
}

func TestDeleteAttemptAPI(t *testing.T) {
	server, workspaces := testRouter(t)

	createResponse, err := http.Post(server.URL+"/attempts", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer createResponse.Body.Close()
	if createResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createResponse.StatusCode, http.StatusCreated)
	}
	var created struct {
		AttemptID string `json:"attemptID"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.AttemptID == "" {
		t.Fatal("create response omitted attemptID")
	}
	if _, err := os.Stat(filepath.Join(workspaces, created.AttemptID, "sample.txt")); err != nil {
		t.Fatalf("created workspace missing challenge file: %v", err)
	}

	deleteResponse := deleteAttempt(t, server.URL, `{"attemptId":"`+created.AttemptID+`"}`)
	if deleteResponse.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d, want %d", deleteResponse.StatusCode, http.StatusOK)
	}
	deleteResponse.Body.Close()
	if _, err := os.Stat(filepath.Join(workspaces, created.AttemptID)); !os.IsNotExist(err) {
		t.Fatalf("workspace should be removed; stat error = %v", err)
	}

	// A repeated delete must not claim success for an already-removed attempt.
	repeated := deleteAttempt(t, server.URL, `{"attemptId":"`+created.AttemptID+`"}`)
	defer repeated.Body.Close()
	if repeated.StatusCode != http.StatusNotFound {
		t.Fatalf("repeated delete status = %d, want %d", repeated.StatusCode, http.StatusNotFound)
	}
}

func TestDeleteAttemptAPIRejectsInvalidRequests(t *testing.T) {
	server, _ := testRouter(t)
	cases := []struct {
		name string
		body string
		want int
	}{
		{name: "malformed JSON", body: `{"attemptId":`, want: http.StatusBadRequest},
		{name: "missing ID", body: `{}`, want: http.StatusBadRequest},
		{name: "empty ID", body: `{"attemptId":""}`, want: http.StatusBadRequest},
		{name: "path traversal", body: `{"attemptId":"../outside"}`, want: http.StatusBadRequest},
		{name: "unknown ID", body: `{"attemptId":"missing-attempt"}`, want: http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := deleteAttempt(t, server.URL, tc.body)
			defer response.Body.Close()
			if response.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", response.StatusCode, tc.want)
			}
			if !strings.Contains(response.Header.Get("Content-Type"), "application/json") {
				t.Fatalf("Content-Type = %q, want application/json", response.Header.Get("Content-Type"))
			}
		})
	}
}

func TestDeleteAttemptAPIRequiresJSONBody(t *testing.T) {
	server, _ := testRouter(t)
	request, err := http.NewRequest(http.MethodDelete, server.URL+"/attempts", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusBadRequest)
	}
}

func deleteAttempt(t *testing.T, url, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodDelete, url+"/attempts", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
