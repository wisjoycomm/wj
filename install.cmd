@echo off
setlocal
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "$ErrorActionPreference='Stop'; & ([scriptblock]::Create((Invoke-WebRequest -UseBasicParsing 'https://github.com/wisjoycomm/wj/releases/latest/download/install.ps1').Content))"
exit /b %errorlevel%
