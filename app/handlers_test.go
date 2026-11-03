package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"multis-go/app"
	"multis-go/models"
	"multis-go/runtime"
)

type dummyFS struct{}

func (d *dummyFS) Read(ctx context.Context, path string) ([]byte, error)        { return []byte("ok"), nil }
func (d *dummyFS) Write(ctx context.Context, path string, data []byte) error    { return nil }
func (d *dummyFS) Exists(ctx context.Context, path string) (bool, error)        { return true, nil }
func (d *dummyFS) List(ctx context.Context, path string) ([]string, error)      { return []string{"file"}, nil }
func (d *dummyFS) Metadata(ctx context.Context, path string) (os.FileInfo, error) { return nil, nil }

type dummyCE struct{}

func (d *dummyCE) Execute(ctx context.Context, command string, args []string, timeout time.Duration) (string, error) {
	return "done", nil
}

type dummyAI struct{}

func (d *dummyAI) StreamCompletion(ctx context.Context, messages []runtime.Message, eventChan chan<- models.SSEEvent) (*runtime.StreamResponse, error) {
	return &runtime.StreamResponse{FinishReason: "stop"}, nil
}

func setupTestServer() (*app.Server, *app.Handler) {
	fs := &dummyFS{}
	ce := &dummyCE{}
	ai := &dummyAI{}
	handler := app.NewHandler(fs, ce, ai)
	server := app.NewServer(":0")
	server.RegisterRoutes(handler)
	return server, handler
}

func TestSessionEndpoints(t *testing.T) {
	_, handler := setupTestServer()

	// 1. Create Session
	body := map[string]any{
		"model":     "deepseek-chat",
		"max_steps": 5,
		"workspace": ".",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/session", bytes.NewReader(bodyBytes))
	rec := httptest.NewRecorder()

	handler.CreateSession(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}

	var session models.Session
	if err := json.NewDecoder(rec.Body).Decode(&session); err != nil {
		t.Fatalf("failed to decode created session: %v", err)
	}

	if session.ID == "" {
		t.Errorf("expected non-empty session ID")
	}

	// 2. Get Session
	getReq := httptest.NewRequest(http.MethodGet, "/session/"+session.ID, nil)
	getReq.SetPathValue("id", session.ID)
	getRec := httptest.NewRecorder()

	handler.GetSession(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", getRec.Code)
	}

	// 3. Delete Session
	delReq := httptest.NewRequest(http.MethodDelete, "/session/"+session.ID, nil)
	delReq.SetPathValue("id", session.ID)
	delRec := httptest.NewRecorder()

	handler.DeleteSession(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content, got %d", delRec.Code)
	}
}
