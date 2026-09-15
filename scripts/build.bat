@echo off
setlocal
chcp 65001 >nul

set "ROOT_DIR=%~dp0.."
pushd "%ROOT_DIR%"
if errorlevel 1 exit /b 1

echo 正在编译 Docker 使用的 Linux amd64 二进制...
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=amd64

if not exist project-user\target mkdir project-user\target
if not exist project-project\target mkdir project-project\target
if not exist project-api\target mkdir project-api\target

go build -trimpath -ldflags="-s -w" -o project-user\target\project-user .\project-user
if errorlevel 1 goto failed
go build -trimpath -ldflags="-s -w" -o project-project\target\project-project .\project-project
if errorlevel 1 goto failed
go build -trimpath -ldflags="-s -w" -o project-api\target\project-api .\project-api
if errorlevel 1 goto failed

echo 编译完成
popd
endlocal
exit /b 0

:failed
echo 编译失败
popd
endlocal
exit /b 1
