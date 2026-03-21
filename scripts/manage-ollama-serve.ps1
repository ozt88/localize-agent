param(
  [ValidateSet("start", "stop", "restart", "status")]
  [string]$Action = "status",
  [int]$Port = 11434,
  [string]$Executable = "C:\Users\DELL\AppData\Local\Programs\Ollama\ollama.exe"
)

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$stateDir = Join-Path $repoRoot "workflow\output\ollama"
$pidPath = Join-Path $stateDir ("ollama-" + $Port + ".json")
$wrapperScript = Join-Path $repoRoot "scripts\run-ollama-serve-wrapper.ps1"

function Get-OllamaRecord {
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

function Remove-OllamaRecord {
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

function Stop-ManagedOllama {
  $record = Get-OllamaRecord
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
  Remove-OllamaRecord
  return $true
}

New-Item -ItemType Directory -Force -Path $stateDir | Out-Null

switch ($Action) {
  "status" {
    $record = Get-OllamaRecord
    if ($null -eq $record) {
      Write-Output "status=not_managed"
      Write-Output ("port_listen=" + ($(if (Test-ListeningPort -ListenPort $Port) { "true" } else { "false" })))
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
    $stopped = Stop-ManagedOllama
    Write-Output ("stopped=" + ($(if ($stopped) { "true" } else { "false" })))
    exit 0
  }
  "restart" {
    [void](Stop-ManagedOllama)
    $Action = "start"
  }
}

if (-not (Test-Path $Executable)) {
  throw "ollama executable not found: $Executable"
}
if (-not (Test-Path $wrapperScript)) {
  throw "ollama wrapper script not found: $wrapperScript"
}

$record = Get-OllamaRecord
if ($null -ne $record) {
  $existing = Get-ProcessByIdSafe -ProcessId ([int]$record.pid)
  if ($null -ne $existing) {
    Write-Output ("status=already_running pid=" + $record.pid)
    exit 0
  }
  Remove-OllamaRecord
}

$runDir = Join-Path $stateDir ("run_" + (Get-Date -Format "yyyyMMdd_HHmmss"))
New-Item -ItemType Directory -Force -Path $runDir | Out-Null
$stdoutLog = Join-Path $runDir "ollama.stdout.log"
$stderrLog = Join-Path $runDir "ollama.stderr.log"

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
  $record = Get-OllamaRecord
  if ($null -ne $record) {
    break
  }
}
$record = Get-OllamaRecord
if ($null -eq $record) {
  throw "managed ollama wrapper did not write a pid record"
}

Write-Output ("status=started pid=" + $record.pid)
Write-Output ("wrapper_pid=" + $wrapperProc.Id)
Write-Output ("stdout_log=" + $stdoutLog)
Write-Output ("stderr_log=" + $stderrLog)
Write-Output ("port_listen=" + ($(if (Test-ListeningPort -ListenPort $Port) { "true" } else { "false" })))
