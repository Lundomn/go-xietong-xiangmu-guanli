package project

import (
	"context"
	"net/http"
	"strings"
	projectmcp "test.com/project-api/pkg/mcp"
	"test.com/project-api/pkg/report"
	common "test.com/project-common"
	"time"

	"github.com/gin-gonic/gin"
)

type taskSummaryRequest struct {
	ProjectCode      string `json:"projectCode" form:"projectCode"`
	ProjectCodeSnake string `json:"project_code" form:"project_code"`
	TaskCode         string `json:"taskCode" form:"taskCode"`
	TaskCodeSnake    string `json:"task_code" form:"task_code"`
	StartDate        string `json:"startDate" form:"startDate"`
	StartDateSnake   string `json:"start_date" form:"start_date"`
	EndDate          string `json:"endDate" form:"endDate"`
	EndDateSnake     string `json:"end_date" form:"end_date"`
	Focus            string `json:"focus" form:"focus"`
}

// taskSummary is the task-detail counterpart to weeklyReport. It intentionally
// uses the same MCP context tools rather than reading the task service again
// from a second AI-specific code path.
func (p *HandlerProject) taskSummary(c *gin.Context) {
	result := &common.Result{}
	var req taskSummaryRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "请求参数格式不正确"))
		return
	}
	projectCode := firstNonEmpty(req.ProjectCode, req.ProjectCodeSnake)
	taskCode := firstNonEmpty(req.TaskCode, req.TaskCodeSnake)
	if projectCode == "" || taskCode == "" {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "projectCode和taskCode不能为空"))
		return
	}
	start, end, err := report.ResolvePeriod(
		firstNonEmpty(req.StartDate, req.StartDateSnake),
		firstNonEmpty(req.EndDate, req.EndDateSnake),
		time.Now(),
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	ctx = withProjectMCPCache(projectmcp.WithCaller(ctx, projectmcp.Caller{
		MemberID:         c.GetInt64("memberId"),
		MemberName:       c.GetString("memberName"),
		OrganizationCode: c.GetString("organizationCode"),
	}))

	overviewValue, err := projectMCPServer.CallTool(ctx, MCPToolProjectOverview, map[string]any{mcpProjectCodeKey: projectCode})
	if err != nil {
		writeWeeklyMCPError(c, result, err)
		return
	}
	var overview mcpProjectOverview
	if err := decodeMCPValue(overviewValue, &overview); err != nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusBadGateway, "项目概况工具返回格式错误"))
		return
	}

	taskValue, err := projectMCPServer.CallTool(ctx, MCPToolProjectTasks, map[string]any{
		mcpProjectCodeKey: projectCode,
		mcpTaskCodeKey:    taskCode,
	})
	if err != nil {
		writeWeeklyMCPError(c, result, err)
		return
	}
	var taskOutput mcpTaskListOutput
	if err := decodeMCPValue(taskValue, &taskOutput); err != nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusBadGateway, "任务列表工具返回格式错误"))
		return
	}
	if len(taskOutput.Tasks) == 0 {
		c.JSON(http.StatusOK, result.Fail(http.StatusNotFound, "任务不存在或当前用户无权访问"))
		return
	}

	activityValue, err := projectMCPServer.CallTool(ctx, MCPToolTaskActivities, map[string]any{
		mcpProjectCodeKey: projectCode,
		mcpTaskCodeKey:    taskCode,
		mcpStartDateKey:   start.Format("2006-01-02"),
		mcpEndDateKey:     end.AddDate(0, 0, -1).Format("2006-01-02"),
	})
	if err != nil {
		writeWeeklyMCPError(c, result, err)
		return
	}
	var activityOutput mcpActivityOutput
	if err := decodeMCPValue(activityValue, &activityOutput); err != nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusBadGateway, "任务动态工具返回格式错误"))
		return
	}

	generated := report.NewGenerator().GenerateTaskSummary(ctx, report.TaskSummaryInput{
		ProjectCode: projectCode,
		ProjectName: overview.Name,
		Task:        reportTasksFromMCP(taskOutput.Tasks)[0],
		Activities:  reportActivitiesFromMCP(activityOutput.Activities),
		Focus:       strings.TrimSpace(req.Focus),
	})
	generated.Orchestrator = "project-api/mcp"
	generated.MCPTools = []string{MCPToolProjectOverview, MCPToolProjectTasks, MCPToolTaskActivities}
	c.JSON(http.StatusOK, result.Success(generated))
}
