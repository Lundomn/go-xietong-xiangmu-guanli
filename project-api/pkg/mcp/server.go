// Package mcp is the project-specific adapter around the official Go MCP SDK.
//
// It keeps the business layer independent from SDK request types while using
// the SDK for protocol validation and Streamable HTTP transport. The same
// registered handlers are callable in-process by project-api's orchestrator.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Caller carries the authenticated project-api identity into a tool call.
// MCP tools must never accept a member id from model-generated arguments.
type Caller struct {
	MemberID         int64
	MemberName       string
	OrganizationCode string
}

type callerContextKey struct{}

// WithCaller attaches the gateway-authenticated caller to a tool context.
func WithCaller(ctx context.Context, caller Caller) context.Context {
	return context.WithValue(ctx, callerContextKey{}, caller)
}

// CallerFromContext returns the caller attached by the authenticated gateway.
func CallerFromContext(ctx context.Context) (Caller, bool) {
	caller, ok := ctx.Value(callerContextKey{}).(Caller)
	return caller, ok && caller.MemberID > 0
}

// Handler is the business implementation of an MCP tool.
type Handler func(context.Context, map[string]any) (any, error)

// Tool is both the discovery metadata and the executable handler.
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handler     Handler
}

// Server is a project-facing tool registry backed by the official Go MCP SDK.
type Server struct {
	name    string
	version string

	mu    sync.RWMutex
	tools map[string]Tool

	sdkServer   *sdkmcp.Server
	httpHandler http.Handler
}

func NewServer(name, version string) *Server {
	name = strings.TrimSpace(name)
	version = strings.TrimSpace(version)
	sdkServer := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    name,
		Version: version,
	}, &sdkmcp.ServerOptions{
		Instructions: "项目协同 MCP 工具只读取当前登录用户有权限访问的项目数据。",
		Capabilities: &sdkmcp.ServerCapabilities{
			Tools: &sdkmcp.ToolCapabilities{ListChanged: false},
		},
	})
	server := &Server{
		name:      name,
		version:   version,
		tools:     make(map[string]Tool),
		sdkServer: sdkServer,
	}
	server.httpHandler = sdkmcp.NewStreamableHTTPHandler(
		func(*http.Request) *sdkmcp.Server { return sdkServer },
		&sdkmcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
	return server
}

// AddTool registers or replaces a tool in both the local registry and the SDK
// server. Tool names are stable API names and are validated at registration.
func (s *Server) AddTool(tool Tool) error {
	if s == nil {
		return errors.New("mcp server is nil")
	}
	tool.Name = strings.TrimSpace(tool.Name)
	if tool.Name == "" {
		return errors.New("mcp tool name is required")
	}
	if tool.Handler == nil {
		return fmt.Errorf("mcp tool %q handler is required", tool.Name)
	}
	if tool.InputSchema == nil {
		tool.InputSchema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	s.mu.Lock()
	s.tools[tool.Name] = tool
	s.mu.Unlock()

	// Use the SDK's raw handler because our tool schemas are already explicit
	// maps. The adapter still returns a standard structured tool result.
	s.sdkServer.AddTool(&sdkmcp.Tool{
		Name:        tool.Name,
		Description: tool.Description,
		InputSchema: tool.InputSchema,
	}, func(ctx context.Context, request *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		arguments := map[string]any{}
		if request != nil && request.Params != nil && len(request.Params.Arguments) > 0 {
			if err := json.Unmarshal(request.Params.Arguments, &arguments); err != nil {
				return &sdkmcp.CallToolResult{
					Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "工具参数不是合法 JSON: " + err.Error()}},
					IsError: true,
				}, nil
			}
		}
		value, err := tool.Handler(ctx, arguments)
		if err != nil {
			return &sdkmcp.CallToolResult{
				Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: err.Error()}},
				IsError: true,
			}, nil
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return &sdkmcp.CallToolResult{
				Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "工具结果无法编码: " + err.Error()}},
				IsError: true,
			}, nil
		}
		return &sdkmcp.CallToolResult{
			Content:           []sdkmcp.Content{&sdkmcp.TextContent{Text: string(encoded)}},
			StructuredContent: value,
		}, nil
	})
	return nil
}

// ToolDefinitions returns a stable, handler-free snapshot for discovery tests
// and for displaying the available project tools in documentation.
func (s *Server) ToolDefinitions() []map[string]any {
	if s == nil {
		return []map[string]any{}
	}
	s.mu.RLock()
	tools := make([]Tool, 0, len(s.tools))
	for _, tool := range s.tools {
		tools = append(tools, tool)
	}
	s.mu.RUnlock()
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	result := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		result = append(result, map[string]any{
			"name":        tool.Name,
			"description": tool.Description,
			"inputSchema": tool.InputSchema,
		})
	}
	return result
}

// CallTool is used by project-api's in-process orchestrator. It goes through
// the same registry as the HTTP endpoint, so the UI and external MCP host
// cannot silently drift apart.
func (s *Server) CallTool(ctx context.Context, name string, arguments map[string]any) (any, error) {
	if s == nil {
		return nil, errors.New("mcp server is nil")
	}
	name = strings.TrimSpace(name)
	s.mu.RLock()
	tool, ok := s.tools[name]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown MCP tool %q", name)
	}
	if arguments == nil {
		arguments = map[string]any{}
	}
	return tool.Handler(ctx, arguments)
}

// ServeHTTP delegates to the official SDK's Streamable HTTP transport.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Accept") == "" {
		r.Header.Set("Accept", "application/json, text/event-stream")
	}
	s.httpHandler.ServeHTTP(w, r)
}
