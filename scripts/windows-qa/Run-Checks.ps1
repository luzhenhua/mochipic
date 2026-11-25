param([switch]$Formats)

$ErrorActionPreference = 'Stop'
$installation = Join-Path $PSScriptRoot 'MochiPic'
$versionPath = (Get-Content -Raw -LiteralPath (Join-Path $installation 'current-runtime')).Trim()
if ($versionPath -cnotmatch '^runtime-[A-Z2-7]{26,100}$') { throw 'Invalid active runtime path' }
$runtime = Join-Path $installation $versionPath
$logs = Join-Path ([IO.Path]::GetTempPath()) ('MochiPic-QA-' + [Guid]::NewGuid().ToString('N'))
[void](New-Item -ItemType Directory -Path $logs)
$summary = Join-Path $logs 'summary.txt'
$env:VIPSHOME = $runtime
$env:FONTCONFIG_FILE = Join-Path $runtime 'etc\fonts\fonts.conf'
$env:MAGICK_CONFIGURE_PATH = Join-Path $runtime 'etc\ImageMagick-7'
$originalPath = $env:PATH
$env:PATH = "$runtime;$env:PATH"
$env:IMAGEFLOW_STRESS = '1'
$env:IMAGEFLOW_CONFIG_DIR = Join-Path $logs 'isolated-config'
$env:MYGO_LOCALE = 'zh-Hans'
$env:IMAGEFLOW_FORMAT_FIXTURES = ''
$env:IMAGEFLOW_ADVANCED_FIXTURES = ''
$env:IMAGEFLOW_RAW_FIXTURES = ''

function Write-Result([string]$Line) {
    Add-Content -LiteralPath $summary -Value $Line
    Write-Host $Line
}

function Invoke-Check([string]$Name, [string]$Program, [string]$WorkDir, [string]$Arguments, [int]$Seconds, [string[]]$RequiredTests) {
    $stdout = Join-Path $logs "$Name.stdout.txt"
    $stderr = Join-Path $logs "$Name.stderr.txt"
    Write-Host "Running $Name..."
    $timer = [Diagnostics.Stopwatch]::StartNew()
    try {
        $test = Start-Process -FilePath $Program -WorkingDirectory $WorkDir -ArgumentList $Arguments `
            -NoNewWindow -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
        if (!$test.WaitForExit($Seconds * 1000)) {
            & "$env:SystemRoot\System32\taskkill.exe" /PID $test.Id /T /F | Out-Null
            Write-Result "FAIL $Name (timed out)"
            return $false
        }
        $elapsed = [Math]::Round($timer.Elapsed.TotalSeconds, 1)
        if ($test.ExitCode -ne 0) {
            Write-Result "FAIL $Name (exit $($test.ExitCode), ${elapsed}s)"
            return $false
        }
        if ($RequiredTests) {
            $report = Get-Content -Raw -LiteralPath $stdout
            foreach ($required in $RequiredTests) {
                if ($report -cnotmatch ('(?m)^--- PASS: ' + [regex]::Escape($required) + ' \(')) {
                    Write-Result "FAIL $Name (no pass recorded for $required)"
                    return $false
                }
            }
        }
        Write-Result "PASS $Name (${elapsed}s)"
        return $true
    } catch {
        Write-Result "FAIL $Name ($_)"
        return $false
    }
}

Write-Result "Windows build $([Environment]::OSVersion.Version), x64=$([Environment]::Is64BitOperatingSystem)"
$testPackages = @('files', 'imaging', 'batch', 'ui', 'winupdate')
$requiredTests = @()
if ($Formats) {
    $testPackages = @('imaging')
    $requiredTests = @('TestCameraRawCorpus', 'TestExtendedStaticFormats', 'TestSVGUsesExportDimensionsAndKeepsAlpha',
        'TestTimedFormatsExportSelectedFrames', 'TestWebPMuxKeepsIdenticalFastFrames',
        'TestAPNGAlphaBlendAndBackgroundDisposal', 'TestTIFFBookPreservesSelectedPageSizesAndAlpha')
    $env:IMAGEFLOW_RAW_FIXTURES = Join-Path $PSScriptRoot 'internal\imaging\raw-fixtures'
    try {
        $manifest = Get-Content -Raw -LiteralPath (Join-Path $PSScriptRoot 'Camera-Fixtures.json') | ConvertFrom-Json
        $samples = @($manifest.PSObject.Properties)
        if ($samples.Count -ne 9) { throw 'Expected nine camera RAW fixtures' }
        foreach ($sample in $samples) {
            $name, $sha = $sample.Value
            if ($name -cnotmatch '^[a-z0-9-]+\.[a-z0-9]+$') { throw 'Invalid camera fixture filename' }
            $file = Join-Path $env:IMAGEFLOW_RAW_FIXTURES $name
            if ((Get-FileHash -Algorithm SHA256 -LiteralPath $file).Hash -ine $sha) {
                throw "SHA-256 mismatch for $name"
            }
        }
        Write-Result 'PASS camera fixtures (nine SHA-256 hashes)'
    } catch {
        Write-Result "FAIL camera fixtures ($_)"
        Write-Host "Logs: $logs"
        exit 1
    }
}
$allPassed = $true
foreach ($name in $testPackages) {
    $exe = Join-Path $runtime "$name.test.exe"
    $workDir = Join-Path $PSScriptRoot "internal\$name"
    if (!(Invoke-Check $name $exe $workDir '-test.v -test.count=1 -test.timeout=10m' 620 $requiredTests)) {
        $allPassed = $false
    }
}

# The actual portable executable must load codecs and render a native window.
$env:VIPSHOME = ''
$env:FONTCONFIG_FILE = ''
$env:MAGICK_CONFIGURE_PATH = ''
$env:PATH = $originalPath
$samples = 'testdata/still.jxl testdata/raster.bmp testdata/apple.heic testdata/timed.avis testdata/different-pages.tiff'
if ($Formats) { $samples += ' raw-fixtures/canon-5d3.cr2 testdata/timed.heics' }
if (!(Invoke-Check 'native-window' (Join-Path $installation 'MochiPic.exe') (Join-Path $PSScriptRoot 'internal\imaging') "--verify-window $samples" 120)) {
    $allPassed = $false
}

Write-Result 'Manual checks: Explorer drag and drop, Chinese IME, DPI, and editor workflow.'
Write-Host "Logs: $logs"
if (!$allPassed) { exit 1 }
