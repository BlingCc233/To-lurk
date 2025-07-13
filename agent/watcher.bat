@echo off

:: 设置你的程序名
set "processName=monitor-agent.exe"
:: 这个路径将在 Go 程序中被动态替换
set "processPath=_PROCESS_PATH_PLACEHOLDER_"

:loop
:: 检查进程是否在运行
tasklist /FI "IMAGENAME eq %processName%" 2>NUL | find /I /N "%processName%">NUL

:: 如果 "find" 命令出错 (即没找到进程)，则 errorlevel 为 1
if "%ERRORLEVEL%"=="1" (
    echo %TIME% - Process not found, starting it...
    start "" "%processPath%"
)

:: 等待10秒后再次检查
timeout /t 10 /nobreak > NUL

goto loop