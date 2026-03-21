param(
  [ValidateSet("start", "stop", "restart", "status")]
  [string]$Action = "status",
  [int]$Port = 8094,
  [string]$Executable = "c:\Users\DELL\Desktop\localize-agent\workflow\bin\go-review.exe",
  [string]$DBBackend = "postgres",
  [string]$DBDsn = "postgres://postgres@127.0.0.1:5433/localize_agent?sslmode=disable"
)

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$stateDir = Join-Path $repoRoot "workflow\output\review"
$pidPath = Join-Path $stateDir ("review-" + $Port + ".json")

function Get-ReviewRecord {
  if (-not (Test-Path $pidPath)) { return $null }
  try { return Get-Content -Path $pidPath -Raw | ConvertFrom-Json }
  catch {
    Remove-Item -Path $pidPath -Force -ErrorAction SilentlyContinue
    return $null
  }
}

function Remove-ReviewRecord {
  Remove-Item -Path $pidPath -Force -ErrorAction SilentlyContinue
}

function Get-ProcessByIdSafe {
  param([int]$ProcessId)
  try { return Get-Process -Id $ProcessId -ErrorAction Stop } catch { return $null }
}

function Test-ListeningPort {
  param([int]$ListenPort)
  $conn = Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue | Where-Object { $_.LocalPort -eq $ListenPort } | Select-Object -First 1
  return $null -ne $conn
}

function Show-Status {
  $record = Get-ReviewRecord
  $proc = $null
  if ($null -ne $record) {
    $proc = Get-ProcessByIdSafe -ProcessId ([int]$record.pid)
  }
  $listening = Test-ListeningPort -ListenPort $Port
  $status = if ($null -ne $proc -and $listening) { "running" } elseif ($null -ne $record) { "stale_record" } else { "not_managed" }
  Write-Output ("status=" + $status)
  Write-Output ("port=" + $Port)
  Write-Output ("port_listen=" + ($(if ($listening) { "true" } else { "false" })))
  if ($null -ne $record) {
    Write-Output ("pid=" + $record.pid)
    Write-Output ("stdout_log=" + $record.stdout_log)
    Write-Output ("stderr_log=" + $record.stderr_log)
  }
}

New-Item -ItemType Directory -Force -Path $stateDir | Out-Null

switch ($Action) {
  "status" {
    Show-Status
    exit 0
  }
  "stop" {
    $record = Get-ReviewRecord
    if ($null -ne $record) {
      $proc = Get-ProcessByIdSafe -ProcessId ([int]$record.pid)
      if ($null -ne $proc) {
        Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
      }
      Remove-ReviewRecord
    }
    Show-Status
    exit 0
  }
  "restart" {
    $record = Get-ReviewRecord
    if ($null -ne $record) {
      $proc = Get-ProcessByIdSafe -ProcessId ([int]$record.pid)
      if ($null -ne $proc) {
        Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
        Start-Sleep -Milliseconds 500
      }
      Remove-ReviewRecord
    }
    $Action = "start"
  }
}

if (-not (Test-Path $Executable)) {
  throw "go-review.exe not found: $Executable"
}

$runDir = Join-Path $stateDir ("run_" + (Get-Date -Format "yyyyMMdd_HHmmss"))
New-Item -ItemType Directory -Force -Path $runDir | Out-Null
$stdoutLog = Join-Path $runDir "go-review.stdout.log"
$stderrLog = Join-Path $runDir "go-review.stderr.log"

$proc = Start-Process -FilePath $Executable -ArgumentList @(
  "--addr", ("127.0.0.1:{0}" -f $Port),
  "--db-backend", $DBBackend,
  "--db-dsn", $DBDsn
) -RedirectStandardOutput $stdoutLog -RedirectStandardError $stderrLog -WorkingDirectory $repoRoot -WindowStyle Hidden -PassThru

@{
  pid = $proc.Id
  port = $Port
  stdout_log = $stdoutLog
  stderr_log = $stderrLog
} | ConvertTo-Json | Set-Content -Path $pidPath -Encoding utf8

for ($i = 0; $i -lt 20; $i++) {
  Start-Sleep -Milliseconds 500
  if (Test-ListeningPort -ListenPort $Port) {
    break
  }
}

Show-Status

