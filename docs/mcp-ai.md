# MCP + AI 编排

项目的 AI 能力采用“事实工具 + 编排层 + 模型 + 本地回退”的链路：

```text
Vue 前端一键操作
      ↓
project-api AI 编排层
      ↓
MCP 工具层
  ├─ get_project_overview
  ├─ get_project_tasks
  ├─ get_task_activities
  ├─ get_project_health
  └─ get_project_risk_evidence
      ↓
可选大模型生成周报/任务总结
      ↓
模型失败或未配置 → 本地规则生成
```

## 为什么这样拆

- MCP 工具只返回当前用户有权限访问的项目事实，不让模型直接访问数据库，也不接受模型传入的 `member_id`。
- 健康度和风险证据由可解释的规则引擎计算，模型只负责组织语言，避免模型臆造项目数字。
- `project-api` 内部直接调用同一套 MCP Handler，外部 MCP Host 通过 HTTP 调用同一套注册表，减少两套逻辑漂移。
- MCP 工具调用使用请求级缓存，周报依次调用五个工具时不会重复读取同一份任务数据。

## MCP HTTP 入口

```text
POST /project/mcp
Authorization: Bearer <登录后的 access token>
Content-Type: application/json
Accept: application/json, text/event-stream
```

入口已经挂在项目鉴权中间件之后。初始化和工具发现示例：

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"project-client","version":"1.0.0"}}}
```

```json
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
```

调用工具示例：

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "get_project_risk_evidence",
    "arguments": {"project_code": "项目编码"}
  }
}
```

项目使用官方 [Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk)，通过 Streamable HTTP 提供 MCP 传输；业务层只依赖本项目自己的工具适配接口。

## 前端能力

- 项目概览页的“AI 生成周报”会调用 `/project/report/weekly`。
- 任务详情页的机器人按钮会调用 `/project/report/task-summary`。
- 返回值中的 `orchestrator`、`mcp_tools` 和 `generated_by` 可用于演示调用链和排查降级原因。
- 未配置模型时仍能返回完整中文内容；模型调用超时、HTTP 错误或空响应时，`generated_by` 为 `local-fallback`。

## 模型配置

MCP 本身不是模型服务，也不强制绑定某一个模型供应商。当前项目保留已有的可选模型适配配置；不配置时直接使用本地规则生成，不影响项目核心功能。模型密钥只在 `project-api` 服务端读取，前端不会接触。

## 代码位置

```text
project-api/pkg/mcp/server.go                 SDK 适配、工具注册和 HTTP 传输
project-api/api/project/mcp_tools.go          项目事实工具和权限上下文
project-api/api/project/weekly_report.go      周报编排
project-api/api/project/task_summary.go       任务总结编排
project-api/pkg/health                        健康度与风险证据规则引擎
project-api/pkg/report                        本地报告、模型调用和回退
```
