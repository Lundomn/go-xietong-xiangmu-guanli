package health

import (
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 9, 20, 12, 0, 0, 0, reportLocation())

func TestAnalyzeReturnsExplainableRiskSnapshot(t *testing.T) {
	snapshot := Analyze(Input{
		ProjectCode: "P001",
		ProjectName: "协同项目",
		Now:         testNow,
		Tasks: []Task{
			{Name: "已完成接口", Done: true, Assignee: "小李", DoneAt: "2026-09-18 10:00:00"},
			{Name: "支付联调", Assignee: "", Priority: 2, CreatedAt: "2026-09-01 10:00:00", EndAt: "2026-09-18 18:00:00"},
			{Name: "补充测试", Assignee: "小王", CreatedAt: "2026-09-01 10:00:00", EndAt: "2026-09-25 18:00:00"},
		},
		Activities: []Activity{{At: "2026-09-10 10:00:00"}},
	})

	if snapshot.Score >= 70 || snapshot.LevelCode != "critical" {
		t.Fatalf("unexpected score level: %+v", snapshot)
	}
	if snapshot.Metrics.OverdueTasks != 1 || snapshot.Metrics.UnassignedTasks != 1 || snapshot.Metrics.HighPriorityOpen != 1 {
		t.Fatalf("unexpected metrics: %+v", snapshot.Metrics)
	}
	if len(snapshot.Evidence) < 3 || len(snapshot.Suggestions) < 3 {
		t.Fatalf("expected explainable evidence and suggestions: %+v", snapshot)
	}
	if snapshot.Method != "rules" || !strings.Contains(snapshot.Summary, "健康度") {
		t.Fatalf("unexpected snapshot metadata: %+v", snapshot)
	}
}

func TestAnalyzeHealthyProject(t *testing.T) {
	snapshot := Analyze(Input{
		ProjectName: "稳定项目",
		Now:         testNow,
		Tasks: []Task{
			{Name: "接口", Done: true, Assignee: "小李", DoneAt: "2026-09-19 10:00:00"},
			{Name: "测试", Assignee: "小王", CreatedAt: "2026-09-19 10:00:00", EndAt: "2026-09-30 18:00:00"},
		},
		Activities: []Activity{{At: "2026-09-20 10:00:00"}},
	})
	if snapshot.Score != 100 || snapshot.LevelCode != "healthy" {
		t.Fatalf("unexpected healthy snapshot: %+v", snapshot)
	}
	if len(snapshot.Evidence) != 0 {
		t.Fatalf("healthy project should have no evidence: %+v", snapshot.Evidence)
	}
}

func TestAnalyzeNoDataIsNotReportedAsCritical(t *testing.T) {
	snapshot := Analyze(Input{ProjectName: "空项目", Now: testNow})
	if snapshot.Score != 60 || snapshot.LevelCode != "unknown" || snapshot.Level != "暂无数据" {
		t.Fatalf("unexpected empty snapshot: %+v", snapshot)
	}
	if len(snapshot.Suggestions) != 1 {
		t.Fatalf("expected one onboarding suggestion: %+v", snapshot.Suggestions)
	}
}
