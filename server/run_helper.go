package main

// buildRunBat はプロジェクトフォルダ内に同梱する run.bat の内容を生成する。
// run.bat は project.toml と同じ階層に置かれるので、カレントディレクトリ = プロジェクトフォルダ。
func buildRunBat() string {
	return `@echo off
cd /d "%~dp0"

echo.
echo  +======================================+
echo  ^|  Run Script                          ^|
echo  +======================================+
echo.

:: Check Python
python --version > nul 2>&1
if errorlevel 1 (
    py --version > nul 2>&1
    if errorlevel 1 (
        echo  [ERROR] Python not found.
        echo  Please install from https://www.python.org
        pause
        exit /b 1
    )
    set PYTHON=py
) else (
    set PYTHON=python
)

:: Check project.toml
if not exist "project.toml" (
    echo  [ERROR] project.toml not found.
    pause
    exit /b 1
)

:: Run
%PYTHON% pyrunner.py .

echo.
pause
`
}
