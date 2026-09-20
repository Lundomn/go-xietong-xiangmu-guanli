package project

import (
	"context"
	"net/http"
	"strings"
	"test.com/project-api/pkg/health"
	common "test.com/project-common"
	"test.com/project-common/errs"
	"test.com/project-grpc/project"
	"test.com/project-grpc/task"
	"time"

	"github.com/gin-gonic/gin"
)

const projectHealthLogPageSize = int64(200)

// projectHealth uses the existing project/task service boundaries. It does not
// read MySQL from the HTTP gateway, keeping the tenant checks identical to the
// project overview and weekly report APIs.
func (p *HandlerProject) projectHealth(c *gin.Context) {
	result := &common.Result{}
	projectCode := strings.TrimSpace(c.PostForm("projectCode"))
	if projectCode == "" {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "projectCode不能为空"))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
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
		c.JSON(http.StatusOK, result.Fail(http.StatusNotFound, "项目不存在"))
		return
	}

	items, err := collectProjectTasks(ctx, projectCode, memberID)
	if err != nil {
		code, message := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, message))
		return
	}

	now := time.Now()
	activities, source := collectHealthActivities(ctx, memberID, projectCode, items, now)
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
	c.JSON(http.StatusOK, result.Success(snapshot))
}

func collectHealthActivities(ctx context.Context, memberID int64, projectCode string, items []*task.TaskMessage, now time.Time) ([]health.Activity, string) {
	response, err := ProjectServiceClient.GetLogBySelfProject(ctx, &project.ProjectRpcMessage{
		MemberId: memberID,
		Page:     1,
		PageSize: projectHealthLogPageSize,
	})
	if err == nil && response != nil {
		activities := make([]health.Activity, 0)
		start := now.Add(-7 * 24 * time.Hour)
		for _, item := range response.List {
			if item == nil || item.ProjectCode != projectCode {
				continue
			}
			if created, ok := taskDate(item.CreateTime); ok && !created.Before(start) && !created.After(now.Add(time.Second)) {
				activities = append(activities, health.Activity{At: item.CreateTime})
			}
		}
		return activities, "project_log"
	}

	// A timeline outage must not make the risk radar unavailable. Task creation
	// and completion events provide a conservative fallback for activity data.
	start := now.Add(-7 * 24 * time.Hour)
	activities := make([]health.Activity, 0)
	for _, item := range items {
		if item == nil {
			continue
		}
		for _, value := range []string{item.CreateTime, item.DoneTime} {
			if created, ok := taskDate(value); ok && !created.Before(start) && !created.After(now.Add(time.Second)) {
				activities = append(activities, health.Activity{At: value})
			}
		}
	}
	return activities, "task_event_fallback"
}

func projectHealthDate(value string) time.Time {
	if parsed, ok := taskDate(value); ok {
		return parsed
	}
	return time.Time{}
}
