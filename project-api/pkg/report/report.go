package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "time/tzdata"
)

const (
	dateLayout     = "2006-01-02"
	dateTimeLayout = "2006-01-02 15:04:05"
	maxPeriodDays  = 31
)

var reportLocation = func() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.UTC
	}
	return location
}()

// Task is the small, presentation-independent task shape used by the report engine.
type Task struct {
	ID          string
	Name        string
	Description string
	Assignee    string
	EndAt       string
	Priority    int
	Done        bool
}

// Activity is a task/project activity in the report period.
type Activity struct {
	TaskName string
	Content  string
	Remark   string
	Actor    string
	At       string
}

// Input contains the data collected from the existing project and task services.
type Input struct {
	ProjectCode string
	ProjectName string
	Start       time.Time
	End         time.Time
	Focus       string
	Tasks       []Task
	Activities  []Activity
}

type Period struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type Stats struct {
	TotalTasks      int     `json:"total_tasks"`
	CompletedTasks  int     `json:"completed_tasks"`
	InProgressTasks int     `json:"in_progress_tasks"`
	OverdueTasks    int     `json:"overdue_tasks"`
	UnassignedTasks int     `json:"unassigned_tasks"`
	ActivityCount   int     `json:"activity_count"`
	CompletionRate  float64 `json:"completion_rate"`
}

// WeeklyReport is returned by the weekly report endpoint.
type WeeklyReport struct {
	ProjectCode string   `json:"project_code"`
	ProjectName string   `json:"project_name"`
	Period      Period   `json:"period"`
	Stats       Stats    `json:"stats"`
	Highlights  []string `json:"highlights"`
	Risks       []string `json:"risks"`
	NextSteps   []string `json:"next_steps"`
	Markdown    string   `json:"markdown"`
	GeneratedBy string   `json:"generated_by"`
}

// ResolvePeriod returns a [start, end) period. With no dates it uses the current
// Monday-Sunday week in Asia/Shanghai. The supplied end date is inclusive for
// API callers and is converted to an exclusive boundary here.
func ResolvePeriod(startDate, endDate string, now time.Time) (time.Time, time.Time, error) {
	now = now.In(reportLocation)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, reportLocation)

	if startDate == "" && endDate == "" {
		daysFromMonday := (int(day.Weekday()) + 6) % 7
		start := day.AddDate(0, 0, -daysFromMonday)
		return start, start.AddDate(0, 0, 7), nil
	}

	start := day
	if startDate != "" {
		parsed, ok := parseDate(startDate)
		if !ok {
			return time.Time{}, time.Time{}, fmt.Errorf("start_date must use YYYY-MM-DD")
		}
		start = parsed
	}

	end := start.AddDate(0, 0, 7)
	if endDate != "" {
		parsed, ok := parseDate(endDate)
		if !ok {
			return time.Time{}, time.Time{}, fmt.Errorf("end_date must use YYYY-MM-DD")
		}
		end = parsed.AddDate(0, 0, 1)
	}

	if !end.After(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("end_date must be after start_date")
	}
	if end.Sub(start) > maxPeriodDays*24*time.Hour {
		return time.Time{}, time.Time{}, fmt.Errorf("report period cannot exceed %d days", maxPeriodDays)
	}
	return start, end, nil
}

// ParseDate accepts the timestamp formats currently emitted by the project
// services, plus RFC3339 and Unix seconds/milliseconds for future clients.
func ParseDate(value string) (time.Time, bool) {
	return parseDate(value)
}

func parseDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if unixValue, err := strconv.ParseInt(value, 10, 64); err == nil {
		if unixValue > 1_000_000_000_000 {
			return time.UnixMilli(unixValue).In(reportLocation), true
		}
		if unixValue > 1_000_000_000 {
			return time.Unix(unixValue, 0).In(reportLocation), true
		}
	}
	for _, layout := range []string{dateTimeLayout, dateLayout} {
		if parsed, err := time.ParseInLocation(layout, value, reportLocation); err == nil {
			return parsed, true
		}
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.In(reportLocation), true
	}
	return time.Time{}, false
}

func InPeriod(value string, start, end time.Time) bool {
	parsed, ok := parseDate(value)
	if !ok {
		return false
	}
	return !parsed.Before(start) && parsed.Before(end)
}

// BuildLocalReport creates a useful report without any external model provider.
// It is also the fallback when a remote model is unavailable.
func BuildLocalReport(input Input) WeeklyReport {
	if input.Start.IsZero() || input.End.IsZero() {
		input.Start, input.End, _ = ResolvePeriod("", "", time.Now())
	}

	completedNames := make([]string, 0)
	overdueTasks := make([]Task, 0)
	highPriorityTasks := make([]Task, 0)
	unassigned := 0
	completed := 0
	for _, item := range input.Tasks {
		if item.Done {
			completed++
			if item.Name != "" {
				completedNames = append(completedNames, item.Name)
			}
		} else {
			if deadline, ok := parseDate(item.EndAt); ok && deadline.Before(input.End) {
				overdueTasks = append(overdueTasks, item)
			}
			if item.Priority >= 2 {
				highPriorityTasks = append(highPriorityTasks, item)
			}
		}
		if strings.TrimSpace(item.Assignee) == "" {
			unassigned++
		}
	}

	sort.SliceStable(overdueTasks, func(i, j int) bool {
		left, _ := parseDate(overdueTasks[i].EndAt)
		right, _ := parseDate(overdueTasks[j].EndAt)
		return left.Before(right)
	})

	stats := Stats{
		TotalTasks:      len(input.Tasks),
		CompletedTasks:  completed,
		InProgressTasks: len(input.Tasks) - completed,
		OverdueTasks:    len(overdueTasks),
		UnassignedTasks: lenUnassigned(input.Tasks),
		ActivityCount:   len(input.Activities),
	}
	if stats.TotalTasks > 0 {
		stats.CompletionRate = math.Round(float64(stats.CompletedTasks)/float64(stats.TotalTasks)*10000) / 100
	}

	report := WeeklyReport{
		ProjectCode: input.ProjectCode,
		ProjectName: input.ProjectName,
		Period: Period{
			Start: input.Start.In(reportLocation).Format(dateLayout),
			End:   input.End.AddDate(0, 0, -1).In(reportLocation).Format(dateLayout),
		},
		Stats:       stats,
		Highlights:  make([]string, 0),
		Risks:       make([]string, 0),
		NextSteps:   make([]string, 0),
		GeneratedBy: "local",
	}

	if stats.TotalTasks == 0 {
		report.Highlights = append(report.Highlights, "本周期暂无任务数据，建议先补充任务和负责人。")
	} else {
		report.Highlights = append(report.Highlights,
			fmt.Sprintf("共 %d 个任务，已完成 %d 个，完成率 %.2f%%。", stats.TotalTasks, stats.CompletedTasks, stats.CompletionRate),
		)
		if len(completedNames) > 0 {
			report.Highlights = append(report.Highlights, "已完成："+strings.Join(limitStrings(completedNames, 5), "、")+"。")
		}
	}
	if stats.ActivityCount > 0 {
		report.Highlights = append(report.Highlights, fmt.Sprintf("本周期记录 %d 条任务动态，项目协作保持活跃。", stats.ActivityCount))
	}

	for _, item := range limitTasks(overdueTasks, 5) {
		deadline := item.EndAt
		if parsed, ok := parseDate(item.EndAt); ok {
			deadline = parsed.Format(dateLayout)
		}
		report.Risks = append(report.Risks, fmt.Sprintf("任务“%s”尚未完成，截止时间为 %s。", taskName(item), deadline))
	}
	if len(highPriorityTasks) > 0 {
		report.Risks = append(report.Risks, fmt.Sprintf("有 %d 个高优先级任务仍未完成，需要优先确认阻塞原因。", len(highPriorityTasks)))
	}
	if stats.UnassignedTasks > 0 {
		report.Risks = append(report.Risks, fmt.Sprintf("有 %d 个任务没有负责人，可能影响推进和统计。", stats.UnassignedTasks))
	}
	if stats.ActivityCount == 0 && stats.TotalTasks > 0 {
		report.Risks = append(report.Risks, "本周期没有记录到任务动态，建议确认进展是否及时同步。")
	}
	if len(report.Risks) == 0 {
		report.Risks = append(report.Risks, "暂未发现明显风险，继续保持任务更新频率。")
	}

	for _, item := range limitTasks(overdueTasks, 3) {
		report.NextSteps = append(report.NextSteps, "优先处理逾期任务：“"+taskName(item)+"”。")
	}
	if len(report.NextSteps) < 3 {
		for _, item := range limitTasks(highPriorityTasks, 3-len(report.NextSteps)) {
			report.NextSteps = append(report.NextSteps, "为高优先级任务“"+taskName(item)+"”补充明确的下一步和截止时间。")
		}
	}
	if len(report.NextSteps) == 0 {
		report.NextSteps = append(report.NextSteps, "确认下周目标，给新增任务补齐负责人、截止时间和验收标准。")
	}

	report.Markdown = renderMarkdown(report, input.Focus)
	return report
}

func lenUnassigned(tasks []Task) int {
	count := 0
	for _, item := range tasks {
		if strings.TrimSpace(item.Assignee) == "" {
			count++
		}
	}
	return count
}

func taskName(item Task) string {
	name := strings.TrimSpace(item.Name)
	if name == "" {
		return "未命名任务"
	}
	return name
}

func limitTasks(tasks []Task, limit int) []Task {
	if len(tasks) <= limit {
		return tasks
	}
	return tasks[:limit]
}

func limitStrings(values []string, limit int) []string {
	if len(values) <= limit {
		return values
	}
	return values[:limit]
}

func renderMarkdown(report WeeklyReport, focus string) string {
	var builder strings.Builder
	focus = strings.TrimSpace(focus)
	builder.WriteString("# 项目周报｜" + report.ProjectName + "\n\n")
	builder.WriteString(fmt.Sprintf("周期：%s 至 %s\n\n", report.Period.Start, report.Period.End))
	if focus != "" {
		builder.WriteString("关注重点：" + focus + "\n\n")
	}
	builder.WriteString("## 关键指标\n\n")
	builder.WriteString(fmt.Sprintf("- 任务总数：%d\n- 已完成：%d\n- 进行中：%d\n- 完成率：%.2f%%\n- 逾期任务：%d\n- 任务动态：%d\n\n",
		report.Stats.TotalTasks,
		report.Stats.CompletedTasks,
		report.Stats.InProgressTasks,
		report.Stats.CompletionRate,
		report.Stats.OverdueTasks,
		report.Stats.ActivityCount,
	))
	builder.WriteString("## 本周亮点\n\n")
	writeItems(&builder, report.Highlights)
	builder.WriteString("\n## 风险与阻塞\n\n")
	writeItems(&builder, report.Risks)
	builder.WriteString("\n## 下周建议\n\n")
	writeItems(&builder, report.NextSteps)
	return strings.TrimSpace(builder.String())
}

func writeItems(builder *strings.Builder, items []string) {
	for _, item := range items {
		builder.WriteString("- ")
		builder.WriteString(item)
		builder.WriteString("\n")
	}
}

type Generator struct {
	client   *http.Client
	provider string
	url      string
	apiKey   string
	model    string
}

// NewGenerator reads optional AI settings. The local generator remains the
// default so the endpoint works without credentials or an external network.
func NewGenerator() *Generator {
	timeout := 20 * time.Second
	if seconds, err := strconv.Atoi(os.Getenv("MS_AI_TIMEOUT_SECONDS")); err == nil && seconds > 0 && seconds <= 120 {
		timeout = time.Duration(seconds) * time.Second
	}
	return &Generator{
		client:   &http.Client{Timeout: timeout},
		provider: strings.ToLower(strings.TrimSpace(os.Getenv("MS_AI_PROVIDER"))),
		url:      strings.TrimSpace(os.Getenv("MS_AI_API_URL")),
		apiKey:   strings.TrimSpace(os.Getenv("MS_AI_API_KEY")),
		model:    strings.TrimSpace(os.Getenv("MS_AI_MODEL")),
	}
}

// Generate uses an OpenAI-compatible chat-completions endpoint when configured;
// every failure falls back to the deterministic local report.
func (g *Generator) Generate(ctx context.Context, input Input) WeeklyReport {
	localReport := BuildLocalReport(input)
	if g == nil || !g.remoteEnabled() {
		return localReport
	}
	markdown, err := g.generateRemote(ctx, input, localReport)
	if err != nil {
		localReport.GeneratedBy = "local-fallback"
		return localReport
	}
	localReport.Markdown = markdown
	localReport.GeneratedBy = "ai"
	return localReport
}

func (g *Generator) remoteEnabled() bool {
	if g.url == "" || g.apiKey == "" || g.model == "" {
		return false
	}
	return g.provider == "openai" || g.provider == "openai-compatible" || g.provider == "remote"
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (g *Generator) generateRemote(ctx context.Context, input Input, localReport WeeklyReport) (string, error) {
	prompt := buildPrompt(input, localReport)
	body, err := json.Marshal(chatRequest{
		Model: g.model,
		Messages: []chatMessage{
			{Role: "system", Content: "你是一名项目管理顾问。请根据给定数据生成一份客观、简洁、可执行的中文项目周报，只输出 Markdown，不要虚构数据。"},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.2,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	resp, err := g.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("AI provider returned HTTP %d", resp.StatusCode)
	}
	var parsed chatResponse
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("AI provider returned an empty report")
	}
	return truncate(parsed.Choices[0].Message.Content, 20000), nil
}

func buildPrompt(input Input, localReport WeeklyReport) string {
	type promptTask struct {
		Name     string `json:"name"`
		Assignee string `json:"assignee,omitempty"`
		EndAt    string `json:"end_at,omitempty"`
		Priority int    `json:"priority"`
		Done     bool   `json:"done"`
	}
	tasks := make([]promptTask, 0, len(input.Tasks))
	for _, item := range input.Tasks {
		tasks = append(tasks, promptTask{
			Name:     truncate(taskName(item), 120),
			Assignee: truncate(item.Assignee, 80),
			EndAt:    truncate(item.EndAt, 40),
			Priority: item.Priority,
			Done:     item.Done,
		})
		if len(tasks) >= 100 {
			break
		}
	}
	activities := make([]Activity, 0, len(input.Activities))
	for _, item := range input.Activities {
		item.Content = truncate(item.Content, 240)
		item.Remark = truncate(item.Remark, 240)
		activities = append(activities, item)
		if len(activities) >= 100 {
			break
		}
	}
	payload := map[string]any{
		"project_name": input.ProjectName,
		"period":       localReport.Period,
		"focus":        input.Focus,
		"stats":        localReport.Stats,
		"tasks":        tasks,
		"activities":   activities,
	}
	encoded, _ := json.MarshalIndent(payload, "", "  ")
	return "请基于以下 JSON 生成周报，必须包含：关键进展、风险/阻塞、下周建议；保持数字与任务名称准确。\n\n" + string(encoded)
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len([]rune(value)) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max]) + "…"
}
