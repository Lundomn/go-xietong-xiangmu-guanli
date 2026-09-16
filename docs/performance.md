# 压测与性能检查

项目提供了可重复的 k6 压测入口。默认只请求健康端点；配置登录令牌后才会增加项目列表和项目详情读请求。

## 运行压测

需要 Docker：

```bash
./scripts/load-test.sh
```

调整请求速率和持续时间：

```bash
RATE=50 DURATION=60s ./scripts/load-test.sh
```

对已登录读链路压测：

```bash
ACCESS_TOKEN='登录后的访问令牌' \
PROJECT_CODE='项目编码' \
BASE_URL='http://host.docker.internal:8088' \
RATE=20 DURATION=60s \
./scripts/load-test.sh
```

脚本默认阈值为：错误率小于 1%、P95 小于 500ms、P99 小于 1s。短信验证码、AI 周报和文件上传不会被默认压测，因为它们分别有短信费用、模型费用和外部/磁盘副作用；如需压测，应使用专用测试账号、模拟短信服务和隔离的 AI 端点。

## 当前服务基线

不依赖额外工具时，可以先用 ApacheBench 对健康检查做快速基线：

```bash
ab -n 1000 -c 50 http://127.0.0.1:8088/health
ab -n 1000 -c 50 http://127.0.0.1:8080/health
```

健康检查只能验证网关/反向代理的并发承载能力，不能代替带数据库、Redis 和鉴权的业务压测。

## 性能优化优先级

1. 为项目列表、任务列表、项目日志和附件列表补齐复合索引，并在慢查询日志中确认实际命中情况。
2. 为 API 增加 Prometheus 指标：请求总数、状态码、P50/P95/P99、gRPC 错误、MySQL/Redis 延迟和上传大小。
3. 对周报汇总、项目统计中的跨服务调用做缓存或批量接口，避免任务数量增长后出现 N+1 查询。
4. 生产附件从本地 Docker volume 迁移到对象存储（COS/OSS/S3 兼容服务），API 只保存元数据并生成短时鉴权下载地址。
5. 增加 CI：Go test/vet/race、前端构建、镜像扫描和轻量 smoke test；压测只在手工或夜间流水线执行。
6. 评估升级 Vue 2、Ant Design Vue 1 和旧版 npm 依赖；当前构建可以通过，但依赖存在 EOL 和安全告警。

## pprof

API 已支持可选 pprof。只在隔离环境开启：

```dotenv
ENABLE_PPROF=1
```

重启 API 后可访问 `http://127.0.0.1:8088/debug/pprof/`，采样完成后立即关闭，避免生产环境暴露调试接口。
