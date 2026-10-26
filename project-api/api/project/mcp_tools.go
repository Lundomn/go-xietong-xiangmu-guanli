package project

import (
	"context"
	"errors"
	"strings"
	"sync"
	"test.com/project-api/pkg/health"
	projectmcp "test.com/project-api/pkg/mcp"
	"test.com/project-api/pkg/report"
	projectpb "test.com/project-grpc/project"
	"test.com/project-grpc/task"
	"time"
)

const (
	mcpProjectCodeKey = "project_code"
	mcpTaskCodeKey    = "task_code"
	mcpStartDateKey   = "start_date"
	mcpEndDateKey     = "end_date"
)

// These names are part of the interview-facing architecture and the external
// MCP contract. Keep them stable when the underlying gRPC implementation moves.
const (
	MCPToolProjectOverview = "get_project_overview"
	MCPToolProjectTasks    = "get_project_tasks"
	MCPToolTaskActivities  = "get_task_activities"
	MCPToolProjectHealth   = "get_project_health"
	MCPToolRiskEvidence    = "get_project_risk_evidence"
)

var weeklyReportMCPTools = []string{
	MCPToolProjectOverview,
	MCPToolProjectTasks,
	MCPToolTaskActivities,
	MCPToolProjectHealth,
	MCPToolRiskEvidence,
}

// projectMCPCache is request-scoped. The weekly report calls several tools in
// sequence, and the cache prevents each tool from re-reading the same project
// and task board through gRPC. External MCP callers remain stateless.
type projectMCPCache struct {
	mu       sync.Mutex
	overview map[string]*projectpb.ProjectDetailMessage
	tasks    map[string][]*task.TaskMessage
	health   map[string]health.Snapshot
}

type projectMCPCacheKey struct{}

func withProjectMCPCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, projectMCPCacheKey{}, &projectMCPCache{
		overview: make(map[string]*projectpb.ProjectDetailMessage),
		tasks:    make(map[string][]*task.TaskMessage),
		health:   make(map[string]health.Snapshot),
	})
}

func projectMCPCacheFromContext(ctx context.Context) *projectMCPCache {
	cache, _ := ctx.Value(projectMCPCacheKey{}).(*projectMCPCache)
	return cache
}

type mcpProjectOverview struct {
	ProjectCode      string  `json:"project_code"`
	Name             string  `json:"name"`
	Description      string  `json:"description"`
	OwnerName        string  `json:"owner_name,omitempty"`
	BeginTime        string  `json:"begin_time,omitempty"`
	EndTime          string  `json:"end_time,omitempty"`
	Schedule         float64 `json:"schedule"`
	TaskBoardTheme   string  `json:"task_board_theme,omitempty"`
	OrganizationCode string  `json:"organization_code,omitempty"`
	Collected        bool    `json:"collected"`
}

type mcpProjectTask struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Assignee    string `json:"assignee,omitempty"`
	Stage       string `json:"stage,omitempty"`
	Status      string `json:"status,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	BeginAt     string `json:"begin_at,omitempty"`
	EndAt       string `json:"end_at,omitempty"`
	DoneAt      string `json:"done_at,omitempty"`
	Priority    int    `json:"priority"`
	Done        bool   `json:"done"`
}

type mcpTaskActivity struct {
	TaskName string `json:"task_name,omitempty"`
	Content  string `json:"content,omitempty"`
	Remark   string `json:"remark,omitempty"`
	Actor    string `json:"actor,omitempty"`
	At       string `json:"at"`
}

type mcpTaskListOutput struct {
	ProjectCode string           `json:"project_code"`
	Total       int              `json:"total"`
	Tasks       []mcpProjectTask `json:"tasks"`
}

type mcpActivityOutput struct {
	ProjectCode string            `json:"project_code"`
	Period      report.Period     `json:"period"`
	Total       int               `json:"total"`
	Activities  []mcpTaskActivity `json:"activities"`
}

type mcpRiskEvidenceOutput struct {
	ProjectCode    string            `json:"project_code"`
	ProjectName    string            `json:"project_name"`
	Score          int               `json:"score"`
	Level          string            `json:"level"`
	LevelCode      string            `json:"level_code"`
	Metrics        health.Metrics    `json:"metrics"`
	Evidence       []health.Evidence `json:"evidence"`
	Suggestions    []string          `json:"suggestions"`
	ActivitySource string            `json:"activity_source,omitempty"`
	GeneratedAt    string            `json:"generated_at"`
}

var projectMCPServer = newProjectMCPServer()

func newProjectMCPServer() *projectmcp.Server {
	server := projectmcp.NewServer("go-xietong-project-api", "1.0.0")
	_ = server.AddTool(projectmcp.Tool{
		Name:        MCPToolProjectOverview,
		Description: "获取当前登录用户有权限访问的项目基本概况、进度和截止时间。",
		InputSchema: projectCodeSchema(),
		Handler:     toolProjectOverview,
	})
	_ = server.AddTool(projectmcp.Tool{
		Name:        MCPToolProjectTasks,
		Description: "获取项目任务列表；可选 task_code 只返回指定任务。",
		InputSchema: projectTaskSchema(),
		Handler:     toolProjectTasks,
	})
	_ = server.AddTool(projectmcp.Tool{
		Name:        MCPToolTaskActivities,
		Description: "获取项目或指定任务在时间范围内的任务动态和协作记录。",
		InputSchema: activitySchema(),
		Handler:     toolTaskActivities,
	})
	_ = server.AddTool(projectmcp.Tool{
		Name:        MCPToolProjectHealth,
		Description: "基于任务、负责人、截止日期和近期动态计算可解释的项目健康度。",
		InputSchema: projectCodeSchema(),
		Handler:     toolProjectHealth,
	})
	_ = server.AddTool(projectmcp.Tool{
		Name:        MCPToolRiskEvidence,
		Description: "返回项目健康度对应的风险证据、指标和建议行动，供模型引用而不是猜测。",
		InputSchema: projectCodeSchema(),
		Handler:     toolProjectRiskEvidence,
	})
	return server
}

func projectCodeSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			mcpProjectCodeKey: map[string]any{"type": "string", "description": "项目编码"},
		},
		"required": []string{mcpProjectCodeKey},
	}
}

func projectTaskSchema() map[string]any {
	schema := projectCodeSchema()
	properties := schema["properties"].(map[string]any)
	properties[mcpTaskCodeKey] = map[string]any{"type": "string", "description": "可选，任务编码"}
	return schema
}

func activitySchema() map[string]any {
	schema := projectTaskSchema()
	properties := schema["properties"].(map[string]any)
	properties[mcpStartDateKey] = map[string]any{"type": "string", "description": "开始日期，YYYY-MM-DD"}
	properties[mcpEndDateKey] = map[string]any{"type": "string", "description": "结束日期，YYYY-MM-DD，包含当天"}
	return schema
}

func toolProjectOverview(ctx context.Context, args map[string]any) (any, error) {
	code, err := requiredProjectCode(args)
	if err != nil {
		return nil, err
	}
	detail, _, err := loadProjectData(ctx, code)
	if err != nil {
		return nil, err
	}
	return mcpProjectOverview{
		ProjectCode:      code,
		Name:             detail.Name,
		Description:      detail.Description,
		OwnerName:        detail.OwnerName,
		BeginTime:        detail.BeginTime,
		EndTime:          detail.EndTime,
		Schedule:         detail.Schedule,
		TaskBoardTheme:   detail.TaskBoardTheme,
		OrganizationCode: detail.OrganizationCode,
		Collected:        detail.Collected == 1,
	}, nil
}

func toolProjectTasks(ctx context.Context, args map[string]any) (any, error) {
	code, err := requiredProjectCode(args)
	if err != nil {
		return nil, err
	}
	_, items, err := loadProjectData(ctx, code)
	if err != nil {
		return nil, err
	}
	filterTaskCode := optionalString(args, mcpTaskCodeKey, "taskCode")
	tasks := make([]mcpProjectTask, 0, len(items))
	for _, item := range items {
		if item == nil || (filterTaskCode != "" && item.Code != filterTaskCode) {
			continue
		}
		tasks = append(tasks, normalizeMCPTask(item))
	}
	return mcpTaskListOutput{ProjectCode: code, Total: len(tasks), Tasks: tasks}, nil
}

func toolTaskActivities(ctx context.Context, args map[string]any) (any, error) {
	code, err := requiredProjectCode(args)
	if err != nil {
		return nil, err
	}
	_, items, err := loadProjectData(ctx, code)
	if err != nil {
		return nil, err
	}
	filterTaskCode := optionalString(args, mcpTaskCodeKey, "taskCode")
	if filterTaskCode != "" {
		filtered := make([]*task.TaskMessage, 0, 1)
		for _, item := range items {
			if item != nil && item.Code == filterTaskCode {
				filtered = append(filtered, item)
				break
			}
		}
		items = filtered
	}
	start, end, err := report.ResolvePeriod(
		optionalString(args, mcpStartDateKey, "startDate"),
		optionalString(args, mcpEndDateKey, "endDate"),
		time.Now(),
	)
	if err != nil {
		return nil, err
	}
	caller, ok := projectmcp.CallerFromContext(ctx)
	if !ok {
		return nil, errors.New("MCP 工具缺少已认证用户")
	}
	activities, err := collectTaskActivities(ctx, items, caller.MemberID, start, end)
	if err != nil {
		return nil, err
	}
	result := make([]mcpTaskActivity, 0, len(activities))
	for _, item := range activities {
		result = append(result, mcpTaskActivity{
			TaskName: item.TaskName,
			Content:  item.Content,
			Remark:   item.Remark,
			Actor:    item.Actor,
			At:       item.At,
		})
	}
	return mcpActivityOutput{
		ProjectCode: code,
		Period: report.Period{
			Start: start.Format("2006-01-02"),
			End:   end.AddDate(0, 0, -1).Format("2006-01-02"),
		},
		Total:      len(result),
		Activities: result,
	}, nil
}

func toolProjectHealth(ctx context.Context, args map[string]any) (any, error) {
	code, err := requiredProjectCode(args)
	if err != nil {
		return nil, err
	}
	snapshot, err := loadHealthSnapshot(ctx, code)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func toolProjectRiskEvidence(ctx context.Context, args map[string]any) (any, error) {
	code, err := requiredProjectCode(args)
	if err != nil {
		return nil, err
	}
	snapshot, err := loadHealthSnapshot(ctx, code)
	if err != nil {
		return nil, err
	}
	return mcpRiskEvidenceOutput{
		ProjectCode:    snapshot.ProjectCode,
		ProjectName:    snapshot.ProjectName,
		Score:          snapshot.Score,
		Level:          snapshot.Level,
		LevelCode:      snapshot.LevelCode,
		Metrics:        snapshot.Metrics,
		Evidence:       snapshot.Evidence,
		Suggestions:    snapshot.Suggestions,
		ActivitySource: snapshot.ActivitySource,
		GeneratedAt:    snapshot.GeneratedAt,
	}, nil
}

func loadProjectData(ctx context.Context, projectCode string) (*projectpb.ProjectDetailMessage, []*task.TaskMessage, error) {
	caller, ok := projectmcp.CallerFromContext(ctx)
	if !ok {
		return nil, nil, errors.New("MCP 工具缺少已认证用户")
	}
	if cache := projectMCPCacheFromContext(ctx); cache != nil {
		cache.mu.Lock()
		detail, detailOK := cache.overview[projectCode]
		items, itemsOK := cache.tasks[projectCode]
		cache.mu.Unlock()
		if detailOK && itemsOK {
			return detail, items, nil
		}
	}
	detail, err := ProjectServiceClient.FindProjectDetail(ctx, &projectpb.ProjectRpcMessage{
		ProjectCode: projectCode,
		MemberId:    caller.MemberID,
	})
	if err != nil {
		return nil, nil, err
	}
	if detail == nil {
		return nil, nil, errors.New("项目服务返回为空")
	}
	items, err := collectProjectTasks(ctx, projectCode, caller.MemberID)
	if err != nil {
		return nil, nil, err
	}
	if cache := projectMCPCacheFromContext(ctx); cache != nil {
		cache.mu.Lock()
		cache.overview[projectCode] = detail
		cache.tasks[projectCode] = items
		cache.mu.Unlock()
	}
	return detail, items, nil
}

func loadHealthSnapshot(ctx context.Context, projectCode string) (health.Snapshot, error) {
	if cache := projectMCPCacheFromContext(ctx); cache != nil {
		cache.mu.Lock()
		snapshot, ok := cache.health[projectCode]
		cache.mu.Unlock()
		if ok {
			return snapshot, nil
		}
	}
	detail, items, err := loadProjectData(ctx, projectCode)
	if err != nil {
		return health.Snapshot{}, err
	}
	caller, ok := projectmcp.CallerFromContext(ctx)
	if !ok {
		return health.Snapshot{}, errors.New("MCP 工具缺少已认证用户")
	}
	now := time.Now()
	activities, source := collectHealthActivities(ctx, caller.MemberID, projectCode, items, now)
	input := health.Input{
		ProjectCode: projectCode,
		ProjectName: detail.Name,
		Tasks:       make([]health.Task, 0, len(items)),
		Activities:  activities,
		ProjectEnd:  projectHealthDate(detail.EndTime),
		Now:         now,
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		assignee := ""
		if item.Executor != nil {
			assignee = item.Executor.Name
		}
		input.Tasks = append(input.Tasks, health.Task{
			ID:        item.Code,
			Name:      item.Name,
			Assignee:  assignee,
			CreatedAt: item.CreateTime,
			DoneAt:    item.DoneTime,
			EndAt:     item.EndTime,
			Priority:  int(item.Pri),
			Done:      taskIsDone(item),
		})
	}
	snapshot := health.Analyze(input)
	snapshot.ActivitySource = source
	if cache := projectMCPCacheFromContext(ctx); cache != nil {
		cache.mu.Lock()
		cache.health[projectCode] = snapshot
		cache.mu.Unlock()
	}
	return snapshot, nil
}

type taskLogResult struct {
	taskName string
	logs     []*task.TaskLog
	err      error
}

func collectTaskActivities(ctx context.Context, tasks []*task.TaskMessage, memberID int64, start, end time.Time) ([]report.Activity, error) {
	if len(tasks) == 0 {
		return []report.Activity{}, nil
	}
	limit := len(tasks)
	if limit > 100 {
		limit = 100
	}
	results := make(chan taskLogResult, limit)
	semaphore := make(chan struct{}, 8)
	var waitGroup sync.WaitGroup
	for _, item := range tasks[:limit] {
		if item == nil || item.Code == "" {
			continue
		}
		waitGroup.Add(1)
		go func(current *task.TaskMessage) {
			defer waitGroup.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			response, err := TaskServiceClient.TaskLog(ctx, &task.TaskReqMessage{
				TaskCode: current.Code,
				All:      1,
				MemberId: memberID,
			})
			if err != nil {
				results <- taskLogResult{taskName: current.Name, err: err}
				return
			}
			if response == nil {
				results <- taskLogResult{taskName: current.Name, logs: []*task.TaskLog{}}
				return
			}
			results <- taskLogResult{taskName: current.Name, logs: response.List}
		}(item)
	}
	go func() {
		waitGroup.Wait()
		close(results)
	}()

	activities := make([]report.Activity, 0)
	for batch := range results {
		if batch.err != nil {
			return nil, batch.err
		}
		for _, item := range batch.logs {
			if item == nil || !report.InPeriod(item.CreateTime, start, end) {
				continue
			}
			actor := ""
			if item.Member != nil {
				actor = item.Member.Name
			}
			activities = append(activities, report.Activity{
				TaskName: batch.taskName,
				Content:  item.Content,
				Remark:   item.Remark,
				Actor:    actor,
				At:       item.CreateTime,
			})
		}
	}
	return activities, nil
}

func normalizeMCPTask(item *task.TaskMessage) mcpProjectTask {
	assignee := ""
	if item.Executor != nil {
		assignee = item.Executor.Name
	}
	return mcpProjectTask{
		ID:          item.Code,
		Name:        item.Name,
		Description: item.Description,
		Assignee:    assignee,
		Stage:       item.StageName,
		Status:      item.StatusText,
		CreatedAt:   item.CreateTime,
		BeginAt:     item.BeginTime,
		EndAt:       item.EndTime,
		DoneAt:      item.DoneTime,
		Priority:    int(item.Pri),
		Done:        taskIsDone(item),
	}
}

func requiredProjectCode(args map[string]any) (string, error) {
	code := optionalString(args, mcpProjectCodeKey, "projectCode")
	if code == "" {
		return "", errors.New("project_code 不能为空")
	}
	return code, nil
}

func optionalString(args map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := args[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
