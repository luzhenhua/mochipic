$ErrorActionPreference = 'Stop'
$env:MOCHIPIC_WINDOWS_UPDATE_QA = $PSScriptRoot
$env:IMAGEFLOW_CONFIG_DIR = Join-Path ([IO.Path]::GetTempPath()) ('MochiPic-Update-QA-' + [Guid]::NewGuid().ToString('N'))
$log = "$env:IMAGEFLOW_CONFIG_DIR.log"
$worker = Join-Path $PSScriptRoot 'updater.test.exe'
Write-Host 'Checking the signed update on isolated copies of MochiPic...'
# Keep native stderr in the log on Windows PowerShell 5.1.
$ErrorActionPreference = 'Continue'
& $worker '-test.v' '-test.count=1' '-test.timeout=5m' '-test.run=^TestWindowsSignedUpdate$' 2>&1 | Tee-Object -FilePath $log
$result = $LASTEXITCODE
Write-Host "Logs: $log"
exit $result
