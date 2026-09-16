# 云服务接入说明

项目中需要外部云服务的功能主要有两项：

| 功能 | 云服务 | 默认行为 |
| --- | --- | --- |
| 注册/登录短信验证码 | 腾讯云短信 API 3.0 | 未配置时不发送；本地演示可返回验证码 |
| AI 项目周报 | OpenAI-compatible Chat Completions | 未配置时使用本地规则生成 |

## 一、腾讯云短信

代码已经接入腾讯云短信 `SendSms`（API 版本 `2021-01-11`），使用 API 3.0 签名，不需要在前端暴露任何密钥。腾讯云控制台需要先完成以下准备：

1. 开通短信服务并创建短信应用，取得 `SmsSdkAppId`。
2. 创建并审核短信签名。
3. 创建并审核正文模板。模板建议只有一个变量，例如：`您的验证码为{1}，15分钟内有效。`。
4. 在访问管理中创建权限受限的 API 密钥，取得 SecretId 和 SecretKey。

官方文档：

- [腾讯云发送短信 SendSms](https://cloud.tencent.com/document/api/382/55981)
- [腾讯云 API 3.0 签名方法](https://cloud.tencent.com/document/product/213/30654)

在项目根目录复制环境变量模板并填写配置：

```bash
cp .env.example .env
```

```dotenv
MS_CAPTCHA_EXPOSE_CODE=0
MS_SMS_PROVIDER=tencent
MS_SMS_SECRET_ID=填写腾讯云SecretId
MS_SMS_SECRET_KEY=填写腾讯云SecretKey
MS_SMS_REGION=ap-guangzhou
MS_SMS_ENDPOINT=https://sms.tencentcloudapi.com
MS_SMS_SDK_APP_ID=140xxxxxxxx
MS_SMS_SIGN_NAME=已审核的短信签名
MS_SMS_TEMPLATE_ID=已审核的模板ID
MS_SMS_COUNTRY_CODE=+86
MS_SMS_SESSION_CONTEXT=go-xietong-xiangmu-guanli
MS_SMS_TIMEOUT_SECONDS=8
```

重启服务：

```bash
docker compose -f docker-compose.deploy.yaml up -d --build project-user api
```

短信发送失败时接口不会返回“发送成功”，Redis 中的验证码也会回滚；用户可看到明确的失败提示。相同手机号 60 秒内不能重复发送，验证码有效期为 15 分钟。生产响应只返回 `{ "sent": true }`，不会返回验证码，日志也不会记录验证码和密钥。

本地没有短信账号时：

```dotenv
MS_SMS_PROVIDER=none
MS_CAPTCHA_EXPOSE_CODE=1
```

此模式只适合开发演示，API 会返回验证码；上线前必须改回 `MS_CAPTCHA_EXPOSE_CODE=0` 并配置真实短信服务。

## 二、AI 周报云模型

项目通过 OpenAI-compatible 接口接入云端模型，API 密钥只在 `project-api` 容器中使用，前端不会接触密钥：

```dotenv
MS_AI_PROVIDER=openai-compatible
MS_AI_API_URL=https://your-provider.example.com/v1/chat/completions
MS_AI_API_KEY=填写云模型密钥
MS_AI_MODEL=填写模型名称
MS_AI_TIMEOUT_SECONDS=20
```

重启 API：

```bash
docker compose -f docker-compose.deploy.yaml up -d --build api
```

云模型不可用、超时或返回格式错误时，接口会自动降级到本地规则周报，并将 `generated_by` 标记为 `local-fallback`。启用云模型后，项目名称、任务和任务动态会发送给对应供应商，正式使用前请确认数据合规、地域和费用限制。

## 安全检查清单

- 不要把 `.env`、SecretKey 或 AI Key 提交到 Git。
- 腾讯云 API 密钥只授予短信发送所需权限，并设置费用/调用频率限制。
- 生产环境保持 HTTPS，并将 Compose 端口限制在内网或本机反向代理之后。
- 先用腾讯云 API Explorer 或测试手机号验证签名、模板变量和地域，再开放注册。
