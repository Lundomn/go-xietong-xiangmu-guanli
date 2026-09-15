# 项目 AI 周报

## 接口

部署后通过 API 网关调用：

```http
POST http://127.0.0.1:8088/project/report/weekly
Authorization: Bearer <登录后获得的 token>
Content-Type: application/json

{
  "projectCode": "项目编码",
  "startDate": "2026-09-07",
  "endDate": "2026-09-13",
  "focus": "重点关注延期风险和下周计划"
}
```

`startDate` 和 `endDate` 可省略，默认生成当前周一至周日的周报；时间跨度最长 31 天。接口会汇总项目任务、完成状态、负责人、截止时间和任务动态，并返回完成率、亮点、风险、下周建议和 Markdown 正文。

## AI 模式

默认使用本地规则生成，不依赖外部模型。需要接入 OpenAI 兼容服务时，在启动 API 容器前设置：

```bash
export MS_AI_PROVIDER=openai-compatible
export MS_AI_API_URL=https://your-ai-service.example.com/v1/chat/completions
export MS_AI_API_KEY=your-api-key
export MS_AI_MODEL=your-model
export MS_AI_TIMEOUT_SECONDS=20
docker compose -f docker-compose.deploy.yaml up -d --build
```

外部模型不可用时会自动退回本地生成，接口仍然返回结果，并将 `generated_by` 标记为 `local-fallback`。
