@echo off
REM run_batch.bat
REM Windows批量启动脚本

echo ==========================================
echo   NM-DPoS 批量节点性能测试
echo ==========================================
echo.

REM 配置参数
set NODE_COUNT=%1
if "%NODE_COUNT%"=="" set NODE_COUNT=10

set START_PORT=%2
if "%START_PORT%"=="" set START_PORT=8000

set SEED_ADDR=%3
if "%SEED_ADDR%"=="" set SEED_ADDR=localhost:9000

set ROUNDS=%4
if "%ROUNDS%"=="" set ROUNDS=30

echo 测试配置:
echo   节点数量: %NODE_COUNT%
echo   端口范围: %START_PORT% - %START_PORT% + %NODE_COUNT%
echo   种子节点: %SEED_ADDR%
echo   运行轮次: %ROUNDS%
echo.

REM 创建日志目录
set LOG_DIR=logs\batch_%date:~0,4%%date:~5,2%%date:~8,2%_%time:~0,2%%time:~3,2%%time:~6,2%
mkdir "%LOG_DIR%" 2>nul

echo 启动 %NODE_COUNT% 个节点...
echo 日志目录: %LOG_DIR%
echo.

REM 批量启动节点
for /l %%i in (0,1,%NODE_COUNT%) do (
    set /a NODE_PORT=%START_PORT% + %%i
    set NODE_ID=BatchNode-%%i

    REM 后台启动节点
    start /B cmd /c "set NODE_PORT=!NODE_PORT! && set SEED_NODES=%SEED_ADDR% && go run main.go -mode standalone -node-id !NODE_ID! -log 1 > %LOG_DIR%\!NODE_ID!.log 2>&1"

    echo   [%%i/%NODE_COUNT%] 启动节点 !NODE_ID! (端口: !NODE_PORT!)
)

echo.
echo 所有节点已启动！
echo.
echo 按任意键停止所有节点...
pause >nul

echo.
echo 正在停止所有节点...
taskkill /F /IM go.exe >nul 2>&1

echo 所有节点已停止
echo.
echo 性能测试完成！
echo 日志文件保存在: %LOG_DIR%
pause
