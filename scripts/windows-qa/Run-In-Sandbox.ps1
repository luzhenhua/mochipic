$ErrorActionPreference = 'Stop'
$sandbox = Join-Path $env:SystemRoot 'System32\WindowsSandbox.exe'
if (!(Test-Path -LiteralPath $sandbox)) {
    throw 'Windows Sandbox is unavailable. This kit leaves Windows features as configured. Use Run-On-Clean-Windows.cmd in a fresh Windows VM or account.'
}
$payload = Join-Path $PSScriptRoot 'Payload'
$work = Join-Path ([IO.Path]::GetTempPath()) ('MochiPic-Sandbox-' + [Guid]::NewGuid().ToString('N'))
$logs = Join-Path $work 'Logs'
[void](New-Item -ItemType Directory -Path $logs -Force)
$hostPayload = [Security.SecurityElement]::Escape($payload)
$hostLogs = [Security.SecurityElement]::Escape($logs)
$config = Join-Path $work 'MochiPic-QA.wsb'
@"
<Configuration>
  <Networking>Disable</Networking>
  <MappedFolders>
    <MappedFolder>
      <HostFolder>$hostPayload</HostFolder>
      <SandboxFolder>C:\MochiPic-QA</SandboxFolder>
      <ReadOnly>true</ReadOnly>
    </MappedFolder>
    <MappedFolder>
      <HostFolder>$hostLogs</HostFolder>
      <SandboxFolder>C:\MochiPic-Logs</SandboxFolder>
      <ReadOnly>false</ReadOnly>
    </MappedFolder>
  </MappedFolders>
  <LogonCommand>
    <Command>powershell.exe -NoProfile -ExecutionPolicy Bypass -File C:\MochiPic-QA\Run-Install-Checks.ps1 -LogDir C:\MochiPic-Logs</Command>
  </LogonCommand>
</Configuration>
"@ | Set-Content -LiteralPath $config -Encoding UTF8
Start-Process -FilePath $config
Write-Host "The clean Windows test will write installer-result.txt and installer.log to: $logs"
$resultFile = Join-Path $logs 'installer-result.txt'
$deadline = [DateTime]::UtcNow.AddMinutes(22)
do {
    if (Test-Path -LiteralPath $resultFile) {
        $result = ([string](Get-Content -Raw -LiteralPath $resultFile -ErrorAction SilentlyContinue)).Trim()
        if ($result -eq 'PASS') { Write-Host 'Installer acceptance passed.'; exit 0 }
        if ($result.StartsWith('FAIL')) { Write-Host "Installer acceptance failed: $result"; exit 1 }
    }
    Start-Sleep -Seconds 2
} while ([DateTime]::UtcNow -lt $deadline)
throw "No completed result arrived from Windows Sandbox. Check the Sandbox window and log folder: $logs"
