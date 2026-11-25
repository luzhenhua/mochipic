param([string]$LogDir)

$ErrorActionPreference = 'Stop'
if (!$LogDir) {
    $LogDir = Join-Path ([IO.Path]::GetTempPath()) ('MochiPic-Install-QA-' + [Guid]::NewGuid().ToString('N'))
}
[void](New-Item -ItemType Directory -Path $LogDir -Force)
$log = Join-Path $LogDir 'installer.log'
$summary = Join-Path $LogDir 'installer-result.txt'
$env:MOCHIPIC_WINDOWS_INSTALL_QA = $PSScriptRoot
$test = Join-Path $PSScriptRoot 'installer.test.exe'
'RUNNING' | Set-Content -LiteralPath $summary -Encoding UTF8
Write-Host "Installer acceptance log: $log"
$ErrorActionPreference = 'Continue'
& $test '-test.v' '-test.count=1' '-test.timeout=20m' '-test.run=^TestInstalledNsisLifecycle$' 2>&1 | Tee-Object -FilePath $log
$result = $LASTEXITCODE
if ($result -eq 0 -and (Select-String -Quiet -SimpleMatch '--- PASS: TestInstalledNsisLifecycle' -LiteralPath $log)) {
    'PASS' | Set-Content -LiteralPath $summary -Encoding UTF8
    Write-Host 'Installer acceptance passed.'
    exit 0
}
"FAIL (exit $result)" | Set-Content -LiteralPath $summary -Encoding UTF8
Write-Host "Installer acceptance failed. Log: $log"
exit 1
