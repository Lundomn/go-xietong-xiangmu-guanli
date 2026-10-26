package project

import (
	"context"
	"strings"
	"testing"

	projectmcp "test.com/project-api/pkg/mcp"
	projectpb "test.com/project-grpc/project"
	"test.com/project-grpc/task"
)

func TestMCPToolsRequireAuthenticatedCaller(t *testing.T) {
	server := newProjectMCPServer()
	for _, name := range weeklyReportMCPTools {
		t.Run(name, func(t *testing.T) {
			_, err := server.CallTool(context.Background(), name, map[string]any{
				"project_code": "P1", "member_id": 7,
			})
			if err == nil || !strings.Contains(err.Error(), "已认证用户") {
				t.Fatalf("model-supplied member_id must not authenticate a caller: %v", err)
			}
		})
	}
}

func TestMCPToolsRejectMissingProjectCode(t *testing.T) {
	server := newProjectMCPServer()
	ctx := projectmcp.WithCaller(context.Background(), projectmcp.Caller{MemberID: 7})
	for _, name := range weeklyReportMCPTools {
		if _, err := server.CallTool(ctx, name, map[string]any{"project_code": " "}); err == nil {
			t.Fatalf("%s accepted an empty project code", name)
		}
	}
}

func TestTaskToolFiltersWithinAuthorizedProjectSnapshot(t *testing.T) {
	ctx := withProjectMCPCache(projectmcp.WithCaller(context.Background(), projectmcp.Caller{MemberID: 7}))
	cache := projectMCPCacheFromContext(ctx)
	cache.overview["P1"] = &projectpb.ProjectDetailMessage{Name: "已授权项目"}
	cache.tasks["P1"] = []*task.TaskMessage{nil, {Code: "T1", Name: "任务一"}, {Code: "T2", Name: "任务二"}}
	for _, tc := range []struct {
		code string
		want int
	}{{"", 2}, {"T1", 1}, {"outside-project", 0}} {
		value, err := toolProjectTasks(ctx, map[string]any{"project_code": "P1", "task_code": tc.code})
		if err != nil {
			t.Fatal(err)
		}
		output := value.(mcpTaskListOutput)
		if output.Total != tc.want || len(output.Tasks) != tc.want {
			t.Fatalf("task filter %q: got %+v", tc.code, output)
		}
		if tc.code == "T1" && output.Tasks[0].ID != "T1" {
			t.Fatalf("returned another task: %+v", output.Tasks)
		}
	}
	other := withProjectMCPCache(projectmcp.WithCaller(context.Background(), projectmcp.Caller{MemberID: 8}))
	if len(projectMCPCacheFromContext(other).tasks) != 0 {
		t.Fatal("request caches must not be shared between callers")
	}
}
