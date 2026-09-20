// Package health contains the explainable project health/risk analysis engine.
//
// The engine intentionally has no HTTP or gRPC dependency. It receives a small
// domain model and returns a deterministic snapshot, which keeps the score
// stable, testable and usable as a fallback when an AI provider is unavailable.
package health

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"test.com/project-api/pkg/report"
	"time"
)

const (
	staleAfter       = 7 * 24 * time.Hour
	inactiveAfter    = 3 * 24 * time.Hour
	deadlineSoon     = 7 * 24 * time.Hour
	maximumRiskItems = 6
)

// Task is the small input shape required by the health engine.
type Task struct {
	ID        string
	Name      string
	Assignee  string
	CreatedAt string
	DoneAt    string
	EndAt     string
	Priority  int
	Done      bool
}

// Activity is an item from the project's collaboration timeline.
type Activity struct {
	At string
}

// Input is deliberately presentation-independent so the engine can be reused
// by the weekly report, API and future scheduled snapshots.
type Input struct {
	ProjectCode string
	ProjectName string
	Tasks       []Task
	Activities  []Activity
	ProjectEnd  time.Time
	Now         time.Time
}

type Metrics struct {
	TotalTasks          int     `json:"total_tasks"`
	CompletedTasks      int     `json:"completed_tasks"`
	InProgressTasks     int     `json:"in_progress_tasks"`
	OverdueTasks        int     `json:"overdue_tasks"`
	UnassignedTasks     int     `json:"unassigned_tasks"`
	HighPriorityOpen    int     `json:"high_priority_open_tasks"`
	StaleOpenTasks      int     `json:"stale_open_tasks"`
	DueWithinSevenDays  int     `json:"due_within_seven_days"`
	RecentActivityCount int     `json:"recent_activity_count"`
	ActivityDaysAgo     int     `json:"activity_days_ago"`
	CompletionRate      float64 `json:"completion_rate"`
}

type Evidence struct {
	Code   string `json:"code"`
	Level  string `json:"level"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Count  int    `json:"count,omitempty"`
}

// Snapshot is the API-facing, explainable result of the rules engine.
type Snapshot struct {
	ProjectCode    string     `json:"project_code"`
	ProjectName    string     `json:"project_name"`
	Score          int        `json:"score"`
	Level          string     `json:"level"`
	LevelCode      string     `json:"level_code"`
	Summary        string     `json:"summary"`
	Metrics        Metrics    `json:"metrics"`
	Evidence       []Evidence `json:"evidence"`
	Suggestions    []string   `json:"suggestions"`
	Method         string     `json:"method"`
	ActivitySource string     `json:"activity_source,omitempty"`
	GeneratedAt    string     `json:"generated_at"`
}

// Analyze computes a stable score from business evidence. The penalties are
// bounded so a single rule cannot turn a small project into an unexplained
// zero. The exact evidence is returned alongside the score for traceability.
func Analyze(input Input) Snapshot {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	now = now.In(reportLocation())

	metrics := Metrics{}
	for _, item := range input.Tasks {
		if item.Done {
			metrics.CompletedTasks++
			continue
		}
		metrics.InProgressTasks++
		if strings.TrimSpace(item.Assignee) == "" {
			metrics.UnassignedTasks++
		}
		if item.Priority >= 2 {
			metrics.HighPriorityOpen++
		}
		if deadline, ok := report.ParseDate(item.EndAt); ok {
			if deadline.Before(now) {
				metrics.OverdueTasks++
			}
			if !deadline.Before(now) && deadline.Before(now.Add(deadlineSoon)) {
				metrics.DueWithinSevenDays++
			}
		}
		if created, ok := report.ParseDate(item.CreatedAt); ok && now.Sub(created) >= staleAfter {
			metrics.StaleOpenTasks++
		}
	}
	metrics.TotalTasks = len(input.Tasks)
	if metrics.TotalTasks > 0 {
		metrics.CompletionRate = round(float64(metrics.CompletedTasks) / float64(metrics.TotalTasks) * 100)
	}

	lastActivity, activityOK := latestActivity(input.Activities)
	if activityOK {
		metrics.RecentActivityCount = len(input.Activities)
		metrics.ActivityDaysAgo = maxInt(0, int(now.Sub(lastActivity).Hours()/24))
	}

	evidence := make([]Evidence, 0, maximumRiskItems)
	penalty := 0
	if metrics.TotalTasks > 0 && metrics.OverdueTasks > 0 {
		overdueRatio := float64(metrics.OverdueTasks) / float64(metrics.InProgressTasks)
		points := minInt(30, maxInt(6, int(math.Ceil(overdueRatio*50))))
		penalty += points
		level := "medium"
		if overdueRatio >= 0.3 || metrics.OverdueTasks >= 5 {
			level = "high"
		}
		evidence = append(evidence, Evidence{
			Code:   "overdue_tasks",
			Level:  level,
			Title:  "存在逾期未完成任务",
			Detail: fmt.Sprintf("%d 个进行中任务中有 %d 个已逾期，逾期比例 %.0f%%。", metrics.InProgressTasks, metrics.OverdueTasks, overdueRatio*100),
			Count:  metrics.OverdueTasks,
		})
	}
	if metrics.HighPriorityOpen > 0 {
		points := minInt(20, metrics.HighPriorityOpen*8)
		penalty += points
		level := "medium"
		if metrics.HighPriorityOpen >= 3 {
			level = "high"
		}
		evidence = append(evidence, Evidence{
			Code:   "high_priority_open",
			Level:  level,
			Title:  "高优先级任务仍在堆积",
			Detail: fmt.Sprintf("有 %d 个高优先级任务尚未完成，建议先确认阻塞原因。", metrics.HighPriorityOpen),
			Count:  metrics.HighPriorityOpen,
		})
	}
	if metrics.UnassignedTasks > 0 {
		points := minInt(15, metrics.UnassignedTasks*4)
		penalty += points
		evidence = append(evidence, Evidence{
			Code:   "unassigned_tasks",
			Level:  "medium",
			Title:  "任务缺少负责人",
			Detail: fmt.Sprintf("有 %d 个进行中任务没有负责人，可能造成推进盲区。", metrics.UnassignedTasks),
			Count:  metrics.UnassignedTasks,
		})
	}
	if metrics.StaleOpenTasks > 0 {
		points := minInt(15, metrics.StaleOpenTasks*3)
		penalty += points
		evidence = append(evidence, Evidence{
			Code:   "stale_tasks",
			Level:  "medium",
			Title:  "任务长期没有完成",
			Detail: fmt.Sprintf("有 %d 个未完成任务创建已超过 7 天，建议拆分或重新评估。", metrics.StaleOpenTasks),
			Count:  metrics.StaleOpenTasks,
		})
	}
	if metrics.TotalTasks > 0 && (!activityOK || now.Sub(lastActivity) >= inactiveAfter) {
		penalty += 15
		detail := "近 7 天没有记录到项目动态。"
		if activityOK {
			detail = fmt.Sprintf("最近一次项目动态距今约 %d 天。", metrics.ActivityDaysAgo)
		}
		evidence = append(evidence, Evidence{
			Code:   "inactive_project",
			Level:  "medium",
			Title:  "项目协作活跃度偏低",
			Detail: detail,
		})
	}
	if !input.ProjectEnd.IsZero() && input.ProjectEnd.After(now) && input.ProjectEnd.Sub(now) <= deadlineSoon && metrics.InProgressTasks > 0 {
		penalty += 10
		evidence = append(evidence, Evidence{
			Code:   "deadline_pressure",
			Level:  "high",
			Title:  "项目临近截止日期",
			Detail: fmt.Sprintf("项目距离截止日期不足 7 天，仍有 %d 个任务未完成。", metrics.InProgressTasks),
		})
	}

	sort.SliceStable(evidence, func(i, j int) bool {
		return evidenceWeight(evidence[i].Level) > evidenceWeight(evidence[j].Level)
	})
	if len(evidence) > maximumRiskItems {
		evidence = evidence[:maximumRiskItems]
	}

	score := 100
	if metrics.TotalTasks == 0 {
		score = 60
	} else {
		score = maxInt(0, 100-minInt(100, penalty))
	}
	levelCode, level := scoreLevel(score, metrics.TotalTasks == 0)
	suggestions := buildSuggestions(evidence)
	if len(suggestions) == 0 {
		suggestions = []string{"继续保持任务、负责人和项目动态的及时更新。"}
	}
	summary := fmt.Sprintf("项目健康度 %d 分，当前为%s。", score, level)
	if metrics.TotalTasks == 0 {
		summary = "项目暂无任务数据，补充任务和负责人后即可开始风险评估。"
	}

	return Snapshot{
		ProjectCode: input.ProjectCode,
		ProjectName: input.ProjectName,
		Score:       score,
		Level:       level,
		LevelCode:   levelCode,
		Summary:     summary,
		Metrics:     metrics,
		Evidence:    evidence,
		Suggestions: suggestions,
		Method:      "rules",
		GeneratedAt: now.Format("2006-01-02 15:04:05"),
	}
}

func buildSuggestions(evidence []Evidence) []string {
	result := make([]string, 0, len(evidence))
	seen := make(map[string]struct{})
	for _, item := range evidence {
		var suggestion string
		switch item.Code {
		case "overdue_tasks":
			suggestion = "先处理逾期任务，逐项确认新的截止时间和阻塞原因。"
		case "high_priority_open":
			suggestion = "为高优先级任务补充明确负责人，并拆成可验收的下一步。"
		case "unassigned_tasks":
			suggestion = "给未分配任务补齐负责人，避免任务进入无人跟进状态。"
		case "stale_tasks":
			suggestion = "检查长期未完成任务，选择拆分、转交或关闭过期任务。"
		case "inactive_project":
			suggestion = "安排一次项目同步，并要求成员在任务动态中记录进展和阻塞。"
		case "deadline_pressure":
			suggestion = "围绕项目截止日期做一次交付盘点，优先锁定剩余关键路径。"
		}
		if suggestion != "" {
			if _, exists := seen[suggestion]; exists {
				continue
			}
			seen[suggestion] = struct{}{}
			result = append(result, suggestion)
		}
	}
	return result
}

func latestActivity(activities []Activity) (time.Time, bool) {
	var latest time.Time
	for _, item := range activities {
		at, ok := report.ParseDate(item.At)
		if ok && (latest.IsZero() || at.After(latest)) {
			latest = at
		}
	}
	return latest, !latest.IsZero()
}

func scoreLevel(score int, noData bool) (string, string) {
	if noData {
		return "unknown", "暂无数据"
	}
	switch {
	case score >= 85:
		return "healthy", "健康"
	case score >= 70:
		return "watch", "关注"
	case score >= 50:
		return "risk", "风险"
	default:
		return "critical", "高风险"
	}
}

func reportLocation() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.UTC
	}
	return location
}

func evidenceWeight(level string) int {
	switch level {
	case "high":
		return 3
	case "medium":
		return 2
	default:
		return 1
	}
}

func round(value float64) float64 {
	return math.Round(value*100) / 100
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
