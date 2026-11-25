@echo off
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0Payload\Run-Install-Checks.ps1"
set "mochipic_qa_exit=%ERRORLEVEL%"
pause
exit /b %mochipic_qa_exit%
