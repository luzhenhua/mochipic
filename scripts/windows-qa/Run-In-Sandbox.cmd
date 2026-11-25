@echo off
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0Run-In-Sandbox.ps1"
set "mochipic_qa_exit=%ERRORLEVEL%"
pause
exit /b %mochipic_qa_exit%
