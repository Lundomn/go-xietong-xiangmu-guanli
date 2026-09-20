package report

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestResolvePeriodDefaultsToMonday(t *testing.T) {
	now := time.Date(2026, 9, 9, 15, 30, 0, 0, reportLocation)
	start, end, err := ResolvePeriod("", "", now)
	if err != nil {
		t.Fatal(err)
	}
	if got := start.Format(dateLayout); got != "2026-09-07" {
		t.Fatalf("start = %s, want Monday 2026-09-07", got)
	}
	if got := end.Format(dateLayout); got != "2026-09-14" {
		t.Fatalf("end = %s, want exclusive 2026-09-14", got)
	}
}

func TestBuildLocalReportFindsOverdueAndUnassignedTasks(t *testing.T) {
	start := time.Date(2026, 9, 7, 0, 0, 0, 0, reportLocation)
	end := start.AddDate(0, 0, 7)
	report := BuildLocalReport(Input{
		ProjectName: "研发协同",
		Start:       start,
		End:         end,
		Tasks: []Task{
			{Name: "完成接口", Done: true, Assignee: "小李", Priority: 1},
			{Name: "补充测试", Done: false, EndAt: "2026-09-08 18:00:00", Priority: 2},
		},
		Activities: []Activity{{TaskName: "完成接口", Content: "已合并", At: "2026-09-08 12:00:00"}},
	})

	if report.Stats.TotalTasks != 2 || report.Stats.CompletedTasks != 1 {
		t.Fatalf("unexpected stats: %+v", report.Stats)
	}
	if report.Stats.OverdueTasks != 1 || report.Stats.UnassignedTasks != 1 {
		t.Fatalf("unexpected risk stats: %+v", report.Stats)
	}
	if report.GeneratedBy != "local" {
		t.Fatalf("GeneratedBy = %q, want local", report.GeneratedBy)
	}
	if !strings.Contains(report.Markdown, "补充测试") || !strings.Contains(report.Markdown, "风险与阻塞") {
		t.Fatalf("markdown did not contain expected content: %s", report.Markdown)
	}
}

func TestBuildLocalReportIncludesHealthContext(t *testing.T) {
	generated := BuildLocalReport(Input{
		ProjectName: "研发协同",
		Health: &HealthSummary{
			Score:     68,
			Level:     "风险",
			LevelCode: "risk",
			Evidence:  []string{"存在逾期未完成任务：有 2 个任务已逾期。"},
		},
	})
	if generated.Health == nil || generated.Health.Score != 68 {
		t.Fatalf("health context was not retained: %+v", generated.Health)
	}
	if !strings.Contains(generated.Markdown, "项目健康度") || !strings.Contains(generated.Markdown, "68/100") {
		t.Fatalf("markdown did not include health context: %s", generated.Markdown)
	}
}

func TestInPeriodSupportsServiceTimestamp(t *testing.T) {
	start := time.Date(2026, 9, 7, 0, 0, 0, 0, reportLocation)
	end := start.AddDate(0, 0, 7)
	if !InPeriod("2026-09-10 10:20:30", start, end) {
		t.Fatal("timestamp should be inside period")
	}
	if InPeriod("2026-09-14 00:00:00", start, end) {
		t.Fatal("exclusive end boundary should be outside period")
	}
}

func TestGeneratorUsesOpenAICompatibleProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("unexpected AI request: method=%s authorization=%q", r.Method, r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"## AI 生成周报\n\n- 已完成接口联调"}}]}`))
	}))
	defer server.Close()
	t.Setenv("MS_AI_PROVIDER", "openai-compatible")
	t.Setenv("MS_AI_API_URL", server.URL)
	t.Setenv("MS_AI_API_KEY", "test-key")
	t.Setenv("MS_AI_MODEL", "test-model")

	generated := NewGenerator().Generate(context.Background(), Input{
		ProjectName: "研发协同",
		Start:       time.Date(2026, 9, 7, 0, 0, 0, 0, reportLocation),
		End:         time.Date(2026, 9, 14, 0, 0, 0, 0, reportLocation),
	})
	if generated.GeneratedBy != "ai" || !strings.Contains(generated.Markdown, "AI 生成周报") {
		t.Fatalf("unexpected AI report: %+v", generated)
	}
}

func TestGeneratorFallsBackWhenProviderFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "provider unavailable", http.StatusBadGateway)
	}))
	defer server.Close()
	t.Setenv("MS_AI_PROVIDER", "openai-compatible")
	t.Setenv("MS_AI_API_URL", server.URL)
	t.Setenv("MS_AI_API_KEY", "test-key")
	t.Setenv("MS_AI_MODEL", "test-model")

	generated := NewGenerator().Generate(context.Background(), Input{ProjectName: "研发协同"})
	if generated.GeneratedBy != "local-fallback" || generated.Markdown == "" {
		t.Fatalf("expected local fallback, got %+v", generated)
	}
}
