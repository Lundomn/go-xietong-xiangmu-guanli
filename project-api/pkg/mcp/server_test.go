package mcp

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServerListsAndCallsToolsOverJSONRPC(t *testing.T) {
	server := NewServer("test-server", "test-version")
	if err := server.AddTool(Tool{
		Name:        "echo_project",
		Description: "echo a project code",
		InputSchema: map[string]any{"type": "object"},
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			caller, ok := CallerFromContext(ctx)
			if !ok || caller.MemberID != 7 {
				t.Fatalf("caller was not propagated: %+v", caller)
			}
			return map[string]any{"project_code": args["project_code"]}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}

	request := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo_project","arguments":{"project_code":"P-1"}}}`
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(request))
	req = req.WithContext(WithCaller(req.Context(), Caller{MemberID: 7}))
	server.ServeHTTP(recorder, req)
	if recorder.Code != 200 {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Result struct {
			StructuredContent map[string]any `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Result.StructuredContent["project_code"] != "P-1" {
		t.Fatalf("unexpected tool result: %+v", response)
	}
}

func TestServerReturnsToolDiscoveryAndNotificationWithoutBody(t *testing.T) {
	server := NewServer("test-server", "test-version")
	if err := server.AddTool(Tool{
		Name:    "noop",
		Handler: func(context.Context, map[string]any) (any, error) { return true, nil },
	}); err != nil {
		t.Fatal(err)
	}

	discovery := httptest.NewRecorder()
	server.ServeHTTP(discovery, httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)))
	if discovery.Code != 200 || !strings.Contains(discovery.Body.String(), `"name":"noop"`) {
		t.Fatalf("unexpected tools/list response: %d %s", discovery.Code, discovery.Body.String())
	}

	notification := httptest.NewRecorder()
	server.ServeHTTP(notification, httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)))
	if notification.Code != 202 || notification.Body.Len() != 0 {
		t.Fatalf("notification should not have a response: %d %q", notification.Code, notification.Body.String())
	}
}
