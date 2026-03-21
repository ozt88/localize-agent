param(
  [string]$ProjectDir = "c:\Users\DELL\Desktop\localize-agent\projects\esoteric-ebb\output\batches\canonical_full_retranslate_dual_score_20260311_1",
  [string]$TargetsPath = "c:\Users\DELL\Desktop\localize-agent\workflow\output\low_score_cluster_targets.json",
  [string]$OutputPath = "c:\Users\DELL\Desktop\localize-agent\workflow\output\low_score_fragment_compare_report.json",
  [string]$ProgressLogPath = "",
  [int]$Limit = 5,
  [int]$TimeoutSec = 90
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$exe = Join-Path $repoRoot "workflow\bin\go-fragment-cluster-retranslate.exe"
if (-not (Test-Path $exe)) {
  throw "fragment cluster executable not found: $exe"
}
if (-not (Test-Path $TargetsPath)) {
  throw "targets file not found: $TargetsPath"
}

$targets = Get-Content $TargetsPath -Raw | ConvertFrom-Json
if ($Limit -gt 0) {
  $targets = @($targets | Select-Object -First $Limit)
}

if ([string]::IsNullOrWhiteSpace($ProgressLogPath)) {
  $ProgressLogPath = [System.IO.Path]::ChangeExtension($OutputPath, ".progress.log")
}

function Write-ProgressLog {
  param([string]$Message)
  $dir = Split-Path -Parent $ProgressLogPath
  if (-not [string]::IsNullOrWhiteSpace($dir)) {
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
  }
  $line = "[{0}] {1}" -f (Get-Date).ToString("o"), $Message
  Add-Content -Path $ProgressLogPath -Value $line -Encoding utf8
}

$results = New-Object System.Collections.Generic.List[object]
$results | ConvertTo-Json -Depth 8 | Set-Content $OutputPath -Encoding utf8
"" | Set-Content $ProgressLogPath -Encoding utf8
$total = @($targets).Count
$index = 0

foreach ($target in $targets) {
  $index++
  $ids = @($target.ids) -join ","
  $clusterName = "{0}-score-{1}-{2}" -f $target.tier, $target.score, $target.ids[0]
  Write-ProgressLog ("[{0}/{1}] start {2}" -f $index, $total, $clusterName)
  $stdoutPath = [System.IO.Path]::GetTempFileName()
  $stderrPath = [System.IO.Path]::GetTempFileName()
  try {
    $proc = Start-Process -FilePath $exe `
      -ArgumentList @("--project-dir", $ProjectDir, "--cluster-name", $clusterName, "--ids", $ids) `
      -WorkingDirectory $repoRoot `
      -RedirectStandardOutput $stdoutPath `
      -RedirectStandardError $stderrPath `
      -WindowStyle Hidden `
      -PassThru

    $finished = $proc.WaitForExit($TimeoutSec * 1000)
    if (-not $finished) {
      try { Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue } catch {}
      $results.Add([pscustomobject]@{
        cluster_name   = $clusterName
        tier           = $target.tier
        score          = $target.score
        ids            = $target.ids
        joined_en      = $target.joined_en
        status         = "timeout"
        timeout_sec    = $TimeoutSec
        before_en      = $null
        before_ko      = $null
        after_ko       = $null
        stderr         = (Get-Content $stderrPath -Raw -ErrorAction SilentlyContinue)
      }) | Out-Null
      $results | ConvertTo-Json -Depth 8 | Set-Content $OutputPath -Encoding utf8
      Write-ProgressLog ("[{0}/{1}] timeout {2}" -f $index, $total, $clusterName)
      continue
    }

    $stdout = Get-Content $stdoutPath -Raw -ErrorAction SilentlyContinue
    $stderr = Get-Content $stderrPath -Raw -ErrorAction SilentlyContinue
    if ($proc.ExitCode -ne 0) {
      $results.Add([pscustomobject]@{
        cluster_name   = $clusterName
        tier           = $target.tier
        score          = $target.score
        ids            = $target.ids
        joined_en      = $target.joined_en
        status         = "error"
        exit_code      = $proc.ExitCode
        before_en      = $null
        before_ko      = $null
        after_ko       = $null
        stderr         = $stderr
        stdout         = $stdout
      }) | Out-Null
      $results | ConvertTo-Json -Depth 8 | Set-Content $OutputPath -Encoding utf8
      Write-ProgressLog ("[{0}/{1}] error {2}" -f $index, $total, $clusterName)
      continue
    }

    $parsed = $stdout | ConvertFrom-Json
    $results.Add([pscustomobject]@{
      cluster_name   = $clusterName
      tier           = $target.tier
      score          = $target.score
      ids            = $target.ids
      joined_en      = $target.joined_en
      status         = "ok"
      before_en      = $parsed.before_en
      before_ko      = $parsed.before_ko
      after_ko       = $parsed.after_ko
      context_before = $parsed.context_before_en
      context_after  = $parsed.context_after_en
    }) | Out-Null
    $results | ConvertTo-Json -Depth 8 | Set-Content $OutputPath -Encoding utf8
    Write-ProgressLog ("[{0}/{1}] ok {2}" -f $index, $total, $clusterName)
  }
  finally {
    Remove-Item $stdoutPath -Force -ErrorAction SilentlyContinue
    Remove-Item $stderrPath -Force -ErrorAction SilentlyContinue
  }
}

$results | ConvertTo-Json -Depth 8 | Set-Content $OutputPath -Encoding utf8
Write-ProgressLog ("complete output={0}" -f $OutputPath)
