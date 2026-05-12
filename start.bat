@echo off
setlocal EnableDelayedExpansion
cd /d "%~dp0"

set EXTERNAL=0
set REBUILD=0
for %%a in (%*) do (
    if "%%a"=="--external" set EXTERNAL=1
    if "%%a"=="-e" set EXTERNAL=1
    if "%%a"=="--rebuild" set REBUILD=1
    if "%%a"=="-r" set REBUILD=1
)

echo.
echo   +======================================+
echo   |     saisei_file_server  起動中...    |
echo   +======================================+
echo.

:: [1/3] pylab-python イメージのビルド（なければ自動ビルド）
docker image inspect pylab-python > nul 2>&1
if errorlevel 1 (
    echo   [1/3] pylab-python イメージをビルドします（初回のみ）...
    copy server\pyrunner.py docker-python\pyrunner.py > nul
    docker build -f docker-python\Dockerfile -t pylab-python docker-python\
    del docker-python\pyrunner.py > nul 2>&1
    if errorlevel 1 (
        echo   [ERROR] pylab-python のビルドに失敗しました
        pause
        exit /b 1
    )
    echo   [1/3] OK ビルド完了
) else (
    echo   [1/3] OK pylab-python イメージは既に存在します
)

:: [2/3] HOST_UPLOAD_DIR を自動設定
for /f "delims=" %%i in ('cd') do set HOST_UPLOAD_DIR=%%i\server\uploads
if not exist "%HOST_UPLOAD_DIR%" mkdir "%HOST_UPLOAD_DIR%"
echo   [2/3] HOST_UPLOAD_DIR = %HOST_UPLOAD_DIR%

:: [3/3] サービス起動
if %EXTERNAL%==1 (
    echo   [3/3] サーバーを外部公開モードで起動します...
    docker compose --profile tunnel up --build -d
) else (
    echo   [3/3] サーバーを起動します...
    docker compose up --build -d
)
if errorlevel 1 (
    echo   [ERROR] 起動に失敗しました
    pause
    exit /b 1
)
echo   [3/3] OK 起動完了

echo.
echo   +-----------------------------------------+
echo   |  OK 起動しました                        |
echo   |  ローカル: http://localhost:4450         |
echo   |  ログ確認: docker compose logs -f       |
echo   |  停止:     docker compose down          |
if %EXTERNAL%==1 (
echo   |  外部URL:  docker compose logs tunnel   |
)
echo   +-----------------------------------------+
echo.
pause
endlocal
