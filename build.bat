@echo off
setlocal
chcp 65001 >nul

echo 请选择要编译的系统环境：
echo 1. Windows amd64
echo 2. Linux amd64
set /p action=请选择：

if "%action%"=="1" goto build_windows
if "%action%"=="2" goto build_linux
echo 无效选项
exit /b 1

:build_windows
echo 编译 Windows amd64 版本
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64
go build -trimpath -ldflags="-s -w" -o project-user\target\project-user.exe .\project-user
go build -trimpath -ldflags="-s -w" -o project-project\target\project-project.exe .\project-project
go build -trimpath -ldflags="-s -w" -o project-api\target\project-api.exe .\project-api
goto done

:build_linux
echo 编译 Linux amd64 版本
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=amd64
go build -trimpath -ldflags="-s -w" -o project-user\target\project-user .\project-user
go build -trimpath -ldflags="-s -w" -o project-project\target\project-project .\project-project
go build -trimpath -ldflags="-s -w" -o project-api\target\project-api .\project-api

:done
echo 编译完成
endlocal
