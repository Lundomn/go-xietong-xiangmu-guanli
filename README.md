# Go 协同项目管理系统

一个基于 Go 微服务架构 + Vue 2 前端的项目协同管理系统，提供项目、任务看板、成员、工时、评论、附件和 AI 周报能力。项目通过 Gin 提供 HTTP API，通过 gRPC 连接用户服务与项目服务，并使用 MySQL、Redis、etcd 完成数据存储、缓存和服务发现。

## 功能概览

- 用户注册、验证码登录、JWT 鉴权
- 个人组织和项目成员管理
- 项目创建、编辑、收藏、归档、回收站与恢复
- 任务阶段、任务创建、任务排序、负责人和任务成员
- 任务动态、评论、工时记录
- 附件上传、乱序分片合并和鉴权下载
- 腾讯云短信验证码生产适配，本地支持演示模式
- 中文本地周报生成，可选接入 OpenAI-compatible 模型
- 项目健康度与风险雷达：规则评分、证据解释和可执行建议，AI 不可用时仍可运行
- Vue 2 + Ant Design Vue 中文工作台，支持项目、任务、附件和 AI 周报操作
- k6 可重复压测、ApacheBench 基线和可选 pprof 性能排查
- Docker Compose 一键启动前端、MySQL、Redis、etcd 和三个 Go 服务
- Kubernetes + Helm 部署包，支持探针、PVC、Ingress 和外部依赖切换

## 技术架构

```text
浏览器
   │ HTTP :8080
   ▼
frontend (Nginx) ──反向代理──> project-api :8088
                                  │
                                  ▼
客户端/脚本 ─────HTTP :8088────── project-api
   ▼
project-api ──gRPC/etcd──> project-user :8881
     │                     project-project :8882
     ├── MySQL
     ├── Redis
     └── 可选 OpenAI-compatible AI 服务
```

目录说明：

```text
frontend          Vue 2 工作台、路由、鉴权、上传和 AI 周报交互
project-api       HTTP 网关、鉴权中间件、文件下载、AI 周报
project-user      用户、组织、验证码和 JWT 服务
project-project   项目、任务、工时、评论和附件元数据服务
project-grpc      gRPC 协议生成代码
project-common    加密、错误码、时间、服务发现和公共运行逻辑
deploy            Dockerfile、服务配置和 MySQL 初始化脚本
deploy/helm       Kubernetes/Helm 部署包
docs              功能说明
```

## 快速启动

需要安装 Docker、Docker Compose 和 Go 1.18 或更高版本。

```bash
cp .env.example .env
# 编辑 .env，至少替换数据库密码和两个 JWT 密钥
./scripts/build.sh
docker compose -f docker-compose.deploy.yaml up -d --build
curl http://127.0.0.1:8080/health
```

Windows 可直接运行 `run.bat`；脚本会先编译 Docker 所需的 Linux amd64 二进制，再启动完整服务。

默认只监听本机地址：

| 服务 | 地址 |
| --- | --- |
| Web 前端 | http://127.0.0.1:8080 |
| HTTP API | http://127.0.0.1:8088 |
| MySQL | 127.0.0.1:3309 |
| Redis | 127.0.0.1:6379 |
| etcd | 127.0.0.1:2379 |

项目服务和用户服务的 gRPC 端口默认只绑定本机，API 网关是对外使用的入口。若部署到服务器，建议在 8088 前增加 HTTPS 反向代理，并根据网络规划调整 Compose 端口绑定。

前端也可以单独开发运行：

```bash
cd frontend
cp .env.example .env
npm install --legacy-peer-deps
npm run serve
```

开发服务器默认使用 `/api/` 代理到 `http://127.0.0.1:8088`。生产容器由 Nginx 提供单页路由、`/api/` 接口代理和鉴权附件下载。

停止服务：

```bash
docker compose -f docker-compose.deploy.yaml down
```

## Kubernetes + Helm 部署

项目同时提供 Helm Chart，适合部署到 Kubernetes。Chart 默认包含前端、API 网关、两个 Go 微服务、MySQL、Redis、etcd、配置、密钥、健康探针和文件上传 PVC，并可选开启 HPA；生产环境可以通过 values 切换到外部 MySQL、Redis、etcd。

完整说明见 [`deploy/helm/go-xietong/README.md`](deploy/helm/go-xietong/README.md)。最小安装示例：

```bash
helm upgrade --install go-xietong ./deploy/helm/go-xietong \
  --namespace go-xietong --create-namespace \
  --set secrets.mysqlRootPassword='change-this-root-password' \
  --set secrets.mysqlPassword='change-this-root-password' \
  --set secrets.jwtAccessSecret='change-this-access-secret' \
  --set secrets.jwtRefreshSecret='change-this-refresh-secret'
```

本地开发仍建议使用 Docker Compose；Kubernetes Chart 的默认依赖是单副本，生产环境应使用托管数据库/缓存、支持 RWX 的文件存储和 HTTPS Ingress。

## 验证接口

```bash
curl http://127.0.0.1:8088/health
curl http://127.0.0.1:8080/health
```

注册和登录接口：

```text
POST /project/login/getCaptcha
POST /project/login/register
POST /project/login
```

登录后携带 `Authorization: Bearer <ACCESS_TOKEN>` 调用项目接口，例如：

```text
POST /project/index
POST /project/project
POST /project/task_stages
POST /project/task/save
POST /project/report/weekly
POST /project/project/health
POST /project/project/_projectStats
POST /project/project/_getProjectReport
POST /project/task/taskDone
POST /project/task/dateTotalForProject
```

验证码默认不会通过 HTTP 返回。没有短信服务的本地演示环境可在 `.env` 中设置 `MS_CAPTCHA_EXPOSE_CODE=1`；生产环境保持为 `0`，并配置腾讯云短信变量。完整步骤见 [`docs/cloud-services.md`](docs/cloud-services.md)。

## AI 周报

接口：

```text
POST /project/report/weekly
```

表单或 JSON 参数支持：

```json
{
  "project_code": "项目编号",
  "start_date": "2026-09-08",
  "end_date": "2026-09-14",
  "focus": "交付风险"
}
```

默认使用本地规则生成器，即使没有外部 AI 也能返回任务统计、进展、风险和下周建议。接入 OpenAI-compatible 服务时配置：

```dotenv
MS_AI_PROVIDER=openai-compatible
MS_AI_API_URL=https://your-ai-endpoint/v1/chat/completions
MS_AI_API_KEY=your-key
MS_AI_MODEL=your-model
```

外部模型只在明确配置后启用；请求失败会自动降级为本地周报。项目名称、任务和动态会发送给配置的模型服务，请根据数据合规要求选择供应商。

## 项目健康度与风险雷达

接口：

```text
POST /project/project/health
```

接口从任务、负责人、截止日期和项目动态计算 0-100 分健康度，并同时返回风险证据和建议行动。评分采用可解释的规则引擎：逾期、高优先级任务、无人负责、长期未更新和临近截止日期分别产生有上限的扣分，避免“黑盒 AI 分数”。项目动态服务异常时会退回任务创建/完成事件，并在 `activity_source` 标记降级来源。

这也是 AI 周报的基础数据层：规则引擎保证事实和数字稳定，AI 只负责把证据转换成中文管理建议，从而降低幻觉、调用成本和不可复现问题。详细字段和面试讲解见 [`docs/health-radar.md`](docs/health-radar.md)。

## 性能与压测

压测脚本、运行参数、基线命令和性能优化优先级见 [`docs/performance.md`](docs/performance.md)。默认压测只覆盖健康检查和可选的只读接口，不会调用短信验证码、AI 周报或文件上传。

## 安全说明

- `.env`、日志、上传文件和构建产物不会提交到 Git。
- 数据库密码和 JWT 密钥只从环境变量读取，示例文件不包含真实凭据。
- 附件下载需要登录，并会再次校验当前用户是否属于对应项目/任务。
- pprof 默认关闭，只有显式设置 `ENABLE_PPROF=1` 才会开放。
- 新注册密码使用 bcrypt；旧版本 MD5 密码在成功登录时自动升级。
- 生产环境应使用 HTTPS、强随机密钥、受限网络和独立数据库账号。

## 测试与检查

本地一键执行静态检查：

```bash
./scripts/verify.sh
```

启动 Docker Compose 后执行完整前后端 Smoke Test：

```bash
MS_CAPTCHA_EXPOSE_CODE=1 ./scripts/smoke.sh
```

Smoke Test 会真实走过健康检查、注册、登录、创建项目、创建任务、单文件上传、乱序分片合并与鉴权下载、项目健康度、AI 周报和项目列表，并在数据库中创建一组带有 `smoke` 前缀的验收数据。验证码明文返回只允许在隔离的本地/CI 环境开启，生产环境必须关闭并接入短信服务。

GitHub Actions 会自动执行 Go 测试、`go vet`、竞态测试、前端构建、Helm 模板校验和 Docker Compose 全链路 Smoke Test。

```bash
export GOPROXY=https://goproxy.cn,direct
export GOSUMDB=off

for module in project-common project-user project-api project-project; do
  (cd "$module" && go test ./...)
  (cd "$module" && go vet ./...)
done
```

AI 周报的本地生成、远程模型成功和远程失败降级均有单元测试；部署验收通过 Smoke Test 覆盖注册、登录、项目、任务、附件下载、项目健康度和周报主链路。

前端构建检查：

```bash
cd frontend
npm run build
```

## License

当前仓库未指定开源许可证。如需公开发布，建议根据使用场景补充 LICENSE 文件。
