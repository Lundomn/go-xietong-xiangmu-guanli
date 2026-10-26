package project

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
	"test.com/project-api/pkg/health"
	projectmcp "test.com/project-api/pkg/mcp"
	"test.com/project-api/pkg/report"
	common "test.com/project-common"
	"test.com/project-common/errs"
	"time"
)

type weeklyReportRequest struct {
	ProjectCode      string `json:"projectCode" form:"projectCode"`
	ProjectCodeSnake string `json:"project_code" form:"project_code"`
	StartDate        string `json:"startDate" form:"startDate"`
	StartDateSnake   string `json:"start_date" form:"start_date"`
	EndDate          string `json:"endDate" form:"endDate"`
	EndDateSnake     string `json:"end_date" form:"end_date"`
	Focus            string `json:"focus" form:"focus"`
}

func (p *HandlerProject) weeklyReport(c *gin.Context) {
	result := &common.Result{}
	var req weeklyReportRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "请求参数格式不正确"))
		return
	}

	projectCode := firstNonEmpty(req.ProjectCode, req.ProjectCodeSnake)
	if projectCode == "" {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "projectCode不能为空"))
		return
	}
	startDate := firstNonEmpty(req.StartDate, req.StartDateSnake)
	endDate := firstNonEmpty(req.EndDate, req.EndDateSnake)
	start, end, err := report.ResolvePeriod(startDate, endDate, time.Now())
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

	overviewValue, err := projectMCPServer.CallTool(ctx, MCPToolProjectOverview, map[string]any{
		mcpProjectCodeKey: projectCode,
	})
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

	activityValue, err := projectMCPServer.CallTool(ctx, MCPToolTaskActivities, map[string]any{
		mcpProjectCodeKey: projectCode,
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

	healthValue, err := projectMCPServer.CallTool(ctx, MCPToolProjectHealth, map[string]any{
		mcpProjectCodeKey: projectCode,
	})
	if err != nil {
		writeWeeklyMCPError(c, result, err)
		return
	}
	var healthSnapshot health.Snapshot
	if err := decodeMCPValue(healthValue, &healthSnapshot); err != nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusBadGateway, "项目健康度工具返回格式错误"))
		return
	}

	// Risk evidence is a separate tool even though it shares the cached health
	// snapshot. This makes the model context explicit and auditable.
	riskValue, riskErr := projectMCPServer.CallTool(ctx, MCPToolRiskEvidence, map[string]any{
		mcpProjectCodeKey: projectCode,
	})
	riskEvidence := mcpRiskEvidenceOutput{}
	if riskErr == nil {
		_ = decodeMCPValue(riskValue, &riskEvidence)
	}
	healthSummary := reportHealthSummary(healthSnapshot, riskEvidence)
	tasks := reportTasksFromMCP(taskOutput.Tasks)
	activities := reportActivitiesFromMCP(activityOutput.Activities)

	generated := report.NewGenerator().Generate(ctx, report.Input{
		ProjectCode: projectCode,
		ProjectName: overview.Name,
		Start:       start,
		End:         end,
		Focus:       strings.TrimSpace(req.Focus),
		Tasks:       tasks,
		Activities:  activities,
		Health:      healthSummary,
	})
	generated.Orchestrator = "project-api/mcp"
	generated.MCPTools = append([]string{}, weeklyReportMCPTools...)
	c.JSON(http.StatusOK, result.Success(generated))
}

func decodeMCPValue(value any, target any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, target)
}

func writeWeeklyMCPError(c *gin.Context, result *common.Result, err error) {
	if grpcCode, message := errs.ParseGrpcError(err); grpcCode != 0 && message != "" {
		c.JSON(http.StatusOK, result.Fail(grpcCode, message))
		return
	}
	c.JSON(http.StatusOK, result.Fail(http.StatusBadGateway, err.Error()))
}

func reportTasksFromMCP(items []mcpProjectTask) []report.Task {
	result := make([]report.Task, 0, len(items))
	for _, item := range items {
		result = append(result, report.Task{
			ID:          item.ID,
			Name:        item.Name,
			Description: item.Description,
			Assignee:    item.Assignee,
			CreatedAt:   item.CreatedAt,
			EndAt:       item.EndAt,
			Priority:    item.Priority,
			Done:        item.Done,
		})
	}
	return result
}

func reportActivitiesFromMCP(items []mcpTaskActivity) []report.Activity {
	result := make([]report.Activity, 0, len(items))
	for _, item := range items {
		result = append(result, report.Activity{
			TaskName: item.TaskName,
			Content:  item.Content,
			Remark:   item.Remark,
			Actor:    item.Actor,
			At:       item.At,
		})
	}
	return result
}

func reportHealthSummary(snapshot health.Snapshot, risk mcpRiskEvidenceOutput) *report.HealthSummary {
	evidence := risk.Evidence
	suggestions := risk.Suggestions
	if len(evidence) == 0 {
		evidence = snapshot.Evidence
	}
	if len(suggestions) == 0 {
		suggestions = snapshot.Suggestions
	}
	evidenceText := make([]string, 0, len(evidence))
	for _, item := range evidence {
		evidenceText = append(evidenceText, item.Title+"："+item.Detail)
	}
	return &report.HealthSummary{
		Score:       snapshot.Score,
		Level:       snapshot.Level,
		LevelCode:   snapshot.LevelCode,
		Evidence:    evidenceText,
		Suggestions: suggestions,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
