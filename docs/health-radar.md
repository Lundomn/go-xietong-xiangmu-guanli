# 项目健康度与风险雷达

## 设计目标

把“AI 生成周报”从一次性文本生成升级成可解释的项目风险闭环：先用真实业务数据得到稳定的健康度和风险证据，再让 AI（可选）负责语言表达和行动建议。

这样做有三个好处：

1. 没有模型密钥、模型超时或模型服务不可用时，项目仍然可以分析。
2. 每个分数都能追溯到任务、负责人、截止时间和动态，不是黑盒结论。
3. 周报、项目概览和未来的定时提醒可以复用同一个分析结果。

## 接口

```http
POST /project/project/health
Authorization: Bearer <登录 token>
Content-Type: application/json

{
  "projectCode": "项目编码"
}
```

返回数据的核心字段：

```json
{
  "score": 68,
  "level": "风险",
  "level_code": "risk",
  "metrics": {
    "total_tasks": 12,
    "completed_tasks": 7,
    "overdue_tasks": 3,
    "unassigned_tasks": 2,
    "stale_open_tasks": 4,
    "completion_rate": 58.33
  },
  "evidence": [
    {
      "code": "overdue_tasks",
      "level": "high",
      "title": "存在逾期未完成任务",
      "detail": "..."
    }
  ],
  "suggestions": ["先处理逾期任务，逐项确认新的截止时间和阻塞原因。"],
  "method": "rules",
  "activity_source": "project_log"
}
```

当前规则覆盖：

- 逾期未完成任务：按比例扣分，扣分有上限；
- 高优先级任务堆积：提示优先处理关键路径；
- 进行中任务没有负责人：提示补齐责任人；
- 创建超过 7 天仍未完成：识别长期停滞任务；
- 近 3 天没有动态：识别协作活跃度下降；
- 项目距离截止日期不足 7 天且仍有未完成任务：提示交付压力。

## 面试讲法

> 我没有简单地把大模型接入项目管理系统，而是设计了“规则引擎 + AI 解释”的项目风险雷达。规则引擎根据任务逾期、负责人、停滞时间和协作动态计算健康度，并返回可追溯的风险证据；AI 只负责把这些事实组织成中文周报和行动建议。这样评分可复现、模型不可用时能降级，也能把同一套风险结果复用到概览、周报和后续提醒中。

## 实现位置

- 规则引擎：`project-api/pkg/health`
- HTTP 接口：`project-api/api/project/project_health.go`
- 概览页：`frontend/src/views/project/space/overview.vue`
- AI 周报：`project-api/pkg/report`，后续可直接接收健康快照作为上下文
