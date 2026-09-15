@echo off
setlocal
chcp 65001 >nul

if not exist .env (
    echo 未找到 .env，请先复制 .env.example 为 .env 并填写密码和 JWT 密钥。
    exit /b 1
)

echo 正在启动 Go 协同项目管理系统...
call "%~dp0scripts\build.bat"
if errorlevel 1 (
    echo Go 服务编译失败，请确认已安装 Go 并检查编译输出。
    exit /b 1
)

docker compose -f docker-compose.deploy.yaml up -d --build
if errorlevel 1 (
    echo 服务启动失败，请检查 Docker 输出。
    exit /b 1
)

echo 服务已启动，健康检查：
curl --fail http://127.0.0.1:8088/health
if errorlevel 1 exit /b 1
endlocal
