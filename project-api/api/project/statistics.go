package project

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	"sort"
	"strings"
	"test.com/project-api/pkg/report"
	common "test.com/project-common"
	"test.com/project-common/errs"
	"test.com/project-grpc/project"
	"test.com/project-grpc/task"
	"time"
)

const projectStatisticsPageSize = int64(200)

// collectProjectTasks reads the task board through the existing task service.
// The API gateway deliberately does not access MySQL directly, so statistics
// use the same tenant and membership checks as the rest of the project UI.
func collectProjectTasks(ctx context.Context, projectCode string, memberID int64) ([]*task.TaskMessage, error) {
	stages, err := TaskServiceClient.TaskStages(ctx, &task.TaskReqMessage{
		ProjectCode: projectCode,
		MemberId:    memberID,
		Page:        1,
		PageSize:    projectStatisticsPageSize,
	})
	if err != nil {
		return nil, err
	}
	if stages == nil {
		return []*task.TaskMessage{}, nil
	}

	items := make([]*task.TaskMessage, 0)
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
			return nil, callErr
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
			items = append(items, item)
		}
	}
	return items, nil
}

func taskIsDone(item *task.TaskMessage) bool {
	return item != nil && (item.Done == 1 || strings.EqualFold(item.ExecuteStatus, "done"))
}

func taskHasAssignee(item *task.TaskMessage) bool {
	return item != nil && item.Executor != nil && strings.TrimSpace(item.Executor.Name) != ""
}

func taskDate(value string) (time.Time, bool) {
	return report.ParseDate(value)
}

func (p *HandlerProject) projectStats(c *gin.Context) {
	result := &common.Result{}
	projectCode := c.PostForm("projectCode")
	if projectCode == "" {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "projectCode不能为空"))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	items, err := collectProjectTasks(ctx, projectCode, c.GetInt64("memberId"))
	if err != nil {
		code, message := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, message))
		return
	}
	now := time.Now()
	location, locationErr := time.LoadLocation("Asia/Shanghai")
	if locationErr == nil {
		now = now.In(location)
	}
	today := now.Format("2006-01-02")
	stats := gin.H{
		"total":       len(items),
		"unDone":      0,
		"done":        0,
		"overdue":     0,
		"toBeAssign":  0,
		"expireToday": 0,
		"doneOverdue": 0,
	}
	for _, item := range items {
		if taskIsDone(item) {
			stats["done"] = stats["done"].(int) + 1
			endAt, endOK := taskDate(item.EndTime)
			doneAt, doneOK := taskDate(item.DoneTime)
			if endOK && doneOK && doneAt.After(endAt) {
				stats["doneOverdue"] = stats["doneOverdue"].(int) + 1
			}
			continue
		}
		stats["unDone"] = stats["unDone"].(int) + 1
		if !taskHasAssignee(item) {
			stats["toBeAssign"] = stats["toBeAssign"].(int) + 1
		}
		if endAt, ok := taskDate(item.EndTime); ok {
			if endAt.Before(now) {
				stats["overdue"] = stats["overdue"].(int) + 1
			}
			if endAt.Format("2006-01-02") == today {
				stats["expireToday"] = stats["expireToday"].(int) + 1
			}
		}
	}
	c.JSON(http.StatusOK, result.Success(stats))
}

func reportDates(start, end time.Time) []time.Time {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.UTC
	}
	start = start.In(location)
	end = end.In(location)
	if start.IsZero() || end.IsZero() || !end.After(start) || end.Sub(start) > 31*24*time.Hour {
		today := time.Now().In(location)
		start = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, location).AddDate(0, 0, -6)
		end = start.AddDate(0, 0, 7)
	}
	dates := make([]time.Time, 0, int(end.Sub(start)/(24*time.Hour))+1)
	for current := start; current.Before(end); current = current.AddDate(0, 0, 1) {
		dates = append(dates, current)
	}
	return dates
}

func buildProjectBurndown(items []*task.TaskMessage, projectStart, projectEnd time.Time) (dates []string, remaining []int, baseline []int) {
	dateList := reportDates(projectStart, projectEnd)
	dates = make([]string, 0, len(dateList))
	remaining = make([]int, 0, len(dateList))
	baseline = make([]int, 0, len(dateList))
	total := len(items)
	for index, day := range dateList {
		dayEnd := day.AddDate(0, 0, 1)
		doneCount := 0
		for _, item := range items {
			if !taskIsDone(item) {
				continue
			}
			doneAt, ok := taskDate(item.DoneTime)
			if ok && doneAt.Before(dayEnd) {
				doneCount++
			}
		}
		remaining = append(remaining, total-doneCount)
		ideal := total
		if len(dateList) > 1 {
			ideal = total - int(float64(total*index)/float64(len(dateList)-1))
		}
		baseline = append(baseline, ideal)
		dates = append(dates, day.Format("2006-01-02"))
	}
	return
}

func (p *HandlerProject) projectReport(c *gin.Context) {
	result := &common.Result{}
	projectCode := c.PostForm("projectCode")
	if projectCode == "" {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "projectCode不能为空"))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	detail, err := ProjectServiceClient.FindProjectDetail(ctx, &project.ProjectRpcMessage{
		ProjectCode: projectCode,
		MemberId:    c.GetInt64("memberId"),
	})
	if err != nil {
		code, message := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, message))
		return
	}
	if detail == nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusNotFound, "项目不存在"))
		return
	}
	items, err := collectProjectTasks(ctx, projectCode, c.GetInt64("memberId"))
	if err != nil {
		code, message := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, message))
		return
	}
	var projectStart, projectEnd time.Time
	if detail.BeginTime != "" {
		projectStart, _ = taskDate(detail.BeginTime)
	}
	if detail.EndTime != "" {
		projectEnd, _ = taskDate(detail.EndTime)
	}
	dates, remaining, baseline := buildProjectBurndown(items, projectStart, projectEnd)
	c.JSON(http.StatusOK, result.Success(gin.H{
		"date":         dates,
		"undoneTask":   remaining,
		"baseLineList": baseline,
	}))
}

func (t *HandlerTask) dateTotalForProject(c *gin.Context) {
	result := &common.Result{}
	projectCode := c.PostForm("projectCode")
	if projectCode == "" {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "projectCode不能为空"))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	items, err := collectProjectTasks(ctx, projectCode, c.GetInt64("memberId"))
	if err != nil {
		code, message := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, message))
		return
	}
	location, locationErr := time.LoadLocation("Asia/Shanghai")
	if locationErr != nil {
		location = time.UTC
	}
	counts := make(map[string]int)
	for _, item := range items {
		if created, ok := taskDate(item.CreateTime); ok {
			key := created.In(location).Format("2006-01-02")
			counts[key]++
		}
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	list := make([]gin.H, 0, len(keys))
	for _, key := range keys {
		list = append(list, gin.H{"date": key, "total": counts[key]})
	}
	c.JSON(http.StatusOK, result.Success(list))
}
