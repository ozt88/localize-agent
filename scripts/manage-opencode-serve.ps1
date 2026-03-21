param(
  [ValidateSet("start", "stop", "restart", "status")]
  [string]$Action = "status",
  [int]$Port = 4112,
  [string]$Executable = "C:\Users\DELL\scoop\apps\opencode\current\opencode.exe"
)

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$stateDir = Join-Path $repoRoot "workflow\output\opencode"
$pidPath = Join-Path $stateDir ("opencode-" + $Port + ".json")
$wrapperScript = Join-Path $repoRoot "scripts\run-opencode-serve-wrapper.ps1"

function Get-OpencodeRecord {
  if (-not (Test-Path $pidPath)) {
    return $null
  }
  try {
    return Get-Content -Path $pidPath -Raw | ConvertFrom-Json
  }
  catch {
    Remove-Item -Path $pidPath -Force -ErrorAction SilentlyContinue
    return $null
  }
}

function Remove-OpencodeRecord {
  Remove-Item -Path $pidPath -Force -ErrorAction SilentlyContinue
}

function Get-ProcessByIdSafe {
  param([int]$ProcessId)
  try {
    return Get-Process -Id $ProcessId -ErrorAction Stop
  }
  catch {
    return $null
  }
}

function Test-ListeningPort {
  param([int]$ListenPort)
  $conn = Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue | Where-Object { $_.LocalPort -eq $ListenPort } | Select-Object -First 1
  return $null -ne $conn
}

function Stop-ManagedOpencode {
  $record = Get-OpencodeRecord
  if ($null -eq $record) {
    return $false
  }
  $proc = Get-ProcessByIdSafe -ProcessId ([int]$record.pid)
  if ($null -ne $proc) {
    try {
      Stop-Process -Id $proc.Id -Force -ErrorAction Stop
      Start-Sleep -Milliseconds 500
    }
    catch {
    }
  }
  Remove-OpencodeRecord
  return $true
}

New-Item -ItemType Directory -Force -Path $stateDir | Out-Null

switch ($Action) {
  "status" {
    $record = Get-OpencodeRecord
    if ($null -eq $record) {
      Write-Output "status=not_managed"
      if (Test-ListeningPort -ListenPort $Port) {
        Write-Output "port_listen=true"
      } else {
        Write-Output "port_listen=false"
      }
      exit 0
    }
    $proc = Get-ProcessByIdSafe -ProcessId ([int]$record.pid)
    Write-Output ("status=" + ($(if ($null -ne $proc) { "running" } else { "stale_record" })))
    Write-Output ("pid=" + $record.pid)
    Write-Output ("port=" + $record.port)
    Write-Output ("exe=" + $record.exe)
    Write-Output ("stdout_log=" + $record.stdout_log)
    Write-Output ("stderr_log=" + $record.stderr_log)
    Write-Output ("port_listen=" + ($(if (Test-ListeningPort -ListenPort $Port) { "true" } else { "false" })))
    exit 0
  }
  "stop" {
    $stopped = Stop-ManagedOpencode
    if ($stopped) {
      Write-Output "stopped=true"
    } else {
      Write-Output "stopped=false"
    }
    exit 0
  }
  "restart" {
    [void](Stop-ManagedOpencode)
    $Action = "start"
  }
}

if (-not (Test-Path $Executable)) {
  throw "opencode executable not found: $Executable"
}
if (-not (Test-Path $wrapperScript)) {
  throw "opencode wrapper script not found: $wrapperScript"
}

$record = Get-OpencodeRecord
if ($null -ne $record) {
  $existing = Get-ProcessByIdSafe -ProcessId ([int]$record.pid)
  if ($null -ne $existing) {
    Write-Output ("status=already_running pid=" + $record.pid)
    exit 0
  }
  Remove-OpencodeRecord
}

$runDir = Join-Path $stateDir ("run_" + (Get-Date -Format "yyyyMMdd_HHmmss"))
New-Item -ItemType Directory -Force -Path $runDir | Out-Null
$stdoutLog = Join-Path $runDir "opencode.stdout.log"
$stderrLog = Join-Path $runDir "opencode.stderr.log"

$wrapperProc = Start-Process -FilePath "powershell.exe" -ArgumentList @(
  "-NoProfile",
  "-ExecutionPolicy", "Bypass",
  "-File", $wrapperScript,
  "-Executable", $Executable,
  "-Port", "$Port",
  "-StdoutLog", $stdoutLog,
  "-StderrLog", $stderrLog,
  "-RecordPath", $pidPath
) -WindowStyle Hidden -PassThru -WorkingDirectory $repoRoot

for ($i = 0; $i -lt 20; $i++) {
  Start-Sleep -Milliseconds 250
  $record = Get-OpencodeRecord
  if ($null -ne $record) {
    break
  }
}
$record = Get-OpencodeRecord

if ($null -eq $record) {
  throw "managed opencode wrapper did not write a pid record"
}

Write-Output ("status=started pid=" + $record.pid)
Write-Output ("wrapper_pid=" + $wrapperProc.Id)
Write-Output ("stdout_log=" + $stdoutLog)
Write-Output ("stderr_log=" + $stderrLog)
Write-Output ("port_listen=" + ($(if (Test-ListeningPort -ListenPort $Port) { "true" } else { "false" })))
