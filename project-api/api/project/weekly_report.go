package project

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
	"sync"
	"test.com/project-api/pkg/report"
	common "test.com/project-common"
	"test.com/project-common/errs"
	"test.com/project-grpc/project"
	"test.com/project-grpc/task"
	"time"
)

const (
	weeklyReportPageSize = int64(200)
	weeklyReportMaxTasks = 100
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
	memberID := c.GetInt64("memberId")
	detail, err := ProjectServiceClient.FindProjectDetail(ctx, &project.ProjectRpcMessage{
		ProjectCode: projectCode,
		MemberId:    memberID,
	})
	if err != nil {
		code, message := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, message))
		return
	}
	if detail == nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusBadGateway, "项目服务返回为空"))
		return
	}

	tasks, activities, err := collectWeeklyData(ctx, projectCode, memberID, start, end)
	if err != nil {
		code, message := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, message))
		return
	}

	generated := report.NewGenerator().Generate(ctx, report.Input{
		ProjectCode: projectCode,
		ProjectName: detail.Name,
		Start:       start,
		End:         end,
		Focus:       strings.TrimSpace(req.Focus),
		Tasks:       tasks,
		Activities:  activities,
	})
	c.JSON(http.StatusOK, result.Success(generated))
}

func collectWeeklyData(ctx context.Context, projectCode string, memberID int64, start, end time.Time) ([]report.Task, []report.Activity, error) {
	stages, err := TaskServiceClient.TaskStages(ctx, &task.TaskReqMessage{
		ProjectCode: projectCode,
		Page:        1,
		PageSize:    weeklyReportPageSize,
		MemberId:    memberID,
	})
	if err != nil {
		return nil, nil, err
	}
	if stages == nil {
		return []report.Task{}, []report.Activity{}, nil
	}

	allTasks := make([]*task.TaskMessage, 0)
	seen := make(map[string]struct{})
	for _, stage := range stages.List {
		if stage == nil || stage.Code == "" {
			continue
		}
		stageTasks, callErr := TaskServiceClient.TaskList(ctx, &task.TaskReqMessage{
			StageCode: stage.Code,
			MemberId:  memberID,
		})
		if callErr != nil {
			return nil, nil, callErr
		}
		if stageTasks == nil {
			continue
		}
		for _, item := range stageTasks.List {
			if item == nil {
				continue
			}
			key := item.Code
			if key == "" {
				key = item.Name + ":" + item.CreateTime
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			allTasks = append(allTasks, item)
		}
	}

	reportTasks := make([]report.Task, 0, len(allTasks))
	for _, item := range allTasks {
		assignee := ""
		if item.Executor != nil {
			assignee = item.Executor.Name
		}
		reportTasks = append(reportTasks, report.Task{
			ID:          item.Code,
			Name:        item.Name,
			Description: item.Description,
			Assignee:    assignee,
			EndAt:       item.EndTime,
			Priority:    int(item.Pri),
			Done:        item.Done == 1 || strings.EqualFold(item.ExecuteStatus, "done"),
		})
	}

	activities, err := collectTaskActivities(ctx, allTasks, memberID, start, end)
	if err != nil {
		return nil, nil, err
	}
	return reportTasks, activities, nil
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
	if limit > weeklyReportMaxTasks {
		limit = weeklyReportMaxTasks
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
