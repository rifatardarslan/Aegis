@echo off
:: Change directory to the script's location
cd /d "%~dp0"
title Aegis Installer
cls
echo ===================================================
echo               *** AEGIS INSTALLER ***
echo ===================================================

:: Check if Go is installed
where go >nul 2>nul
if %errorlevel% neq 0 (
    :: Check common default installation paths
    if exist "C:\Program Files\Go\bin\go.exe" (
        set "PATH=%PATH%;C:\Program Files\Go\bin"
        goto :go_found
    )
    if exist "C:\Go\bin\go.exe" (
        set "PATH=%PATH%;C:\Go\bin"
        goto :go_found
    )
    
    echo [!] Go is not installed or not found in PATH.
    echo [!] Attempting to install GoLang automatically via winget...
    winget install GoLang.Go --silent --accept-source-agreements --accept-package-agreements
    if %errorlevel% equ 0 (
        echo [v] Go installed successfully. Updating PATH...
        set "PATH=%PATH%;C:\Program Files\Go\bin;%USERPROFILE%\go\bin"
        goto :go_found
    )
    
    echo [!] winget installation failed or skipped.
    echo [!] Attempting to download the latest precompiled aegis.exe...
    
    :: Attempt to download from GitHub Releases
    powershell -Command "[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12; Invoke-WebRequest -Uri 'https://github.com/rifatardarslan/Aegis/releases/latest/download/aegis.exe' -OutFile 'aegis.exe'" >nul 2>nul
    
    if exist "aegis.exe" (
        echo [v] Precompiled aegis.exe downloaded or found locally.
        goto :install_binary_only
    )
    
    color 0c
    echo [X] Could not install Go automatically and could not find/download aegis.exe.
    echo [X] Please install Go manually from https://go.dev/dl/
    pause
    exit /b 1
)

:go_found
echo [!] Compiling and verifying with Go...
go install ./cmd/aegis
if %errorlevel% neq 0 (
    color 0c
    echo [X] Go build failed. Checking for precompiled binary...
    if exist "aegis.exe" (
        echo [!] Found precompiled aegis.exe in this folder. Proceeding with it...
        goto :install_binary_only
    )
    pause
    exit /b %errorlevel%
)

echo [!] Fetching GOPATH...
for /f "tokens=*" %%i in ('go env GOPATH') do set "GOPATH_DIR=%%i"
if "%GOPATH_DIR%"=="" set "GOPATH_DIR=%USERPROFILE%\go"

set "BIN_DIR=%GOPATH_DIR%\bin"
if not exist "%BIN_DIR%" mkdir "%BIN_DIR%"

:: If built with go install, aegis.exe should be in GOPATH\bin
if not exist "%BIN_DIR%\aegis.exe" (
    if exist "aegis.exe" (
        copy "aegis.exe" "%BIN_DIR%\aegis.exe" >nul
    ) else if exist "cmd\aegis\aegis.exe" (
        copy "cmd\aegis\aegis.exe" "%BIN_DIR%\aegis.exe" >nul
    )
)
goto :create_launcher

:install_binary_only
echo [!] Installing precompiled binary...
set "BIN_DIR=%USERPROFILE%\.aegis\bin"
if not exist "%BIN_DIR%" mkdir "%BIN_DIR%"
copy /Y "aegis.exe" "%BIN_DIR%\aegis.exe" >NUL

:: Add directory to User PATH permanently using PowerShell
echo [!] Adding Aegis binary folder to User PATH...
powershell -Command "$p = [Environment]::GetEnvironmentVariable('PATH', 'User'); if ($p -notlike '*\.aegis\bin*') { [Environment]::SetEnvironmentVariable('PATH', $p + ';%USERPROFILE%\.aegis\bin', 'User') }"
set "PATH=%PATH%;%USERPROFILE%\.aegis\bin"

:create_launcher
echo [!] Configuring global Device Guard/WDAC bypass wrapper...
set "LAUNCHER_PATH=%BIN_DIR%\aegis.bat"
(
echo @echo off
echo "%BIN_DIR%\aegis.exe" %%*
) > "%LAUNCHER_PATH%"

echo ===================================================
echo [v] AEGIS SUCCESSFULLY INSTALLED!
echo ===================================================
echo [i] You can now type 'aegis' from any folder/new terminal to run!
echo [i] Device Guard/WDAC bypass has been configured.
echo ===================================================
pause
exit /b 0
