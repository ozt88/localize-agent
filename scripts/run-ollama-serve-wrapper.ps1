param(
  [string]$Executable,
  [int]$Port = 11434,
  [string]$StdoutLog,
  [string]$StderrLog,
  [string]$RecordPath
)

$ErrorActionPreference = "Stop"

function Write-TimestampedLine {
  param(
    [string]$Path,
    [string]$Line
  )
  $timestamp = (Get-Date).ToString("o")
  Add-Content -Path $Path -Value ("[{0}] {1}" -f $timestamp, $Line) -Encoding utf8
}

if ([string]::IsNullOrWhiteSpace($Executable) -or -not (Test-Path $Executable)) {
  throw "ollama executable not found: $Executable"
}

$stdoutDir = Split-Path -Parent $StdoutLog
$stderrDir = Split-Path -Parent $StderrLog
if (-not [string]::IsNullOrWhiteSpace($stdoutDir)) {
  New-Item -ItemType Directory -Force -Path $stdoutDir | Out-Null
}
if (-not [string]::IsNullOrWhiteSpace($stderrDir)) {
  New-Item -ItemType Directory -Force -Path $stderrDir | Out-Null
}
New-Item -ItemType File -Force -Path $StdoutLog | Out-Null
New-Item -ItemType File -Force -Path $StderrLog | Out-Null

$psi = New-Object System.Diagnostics.ProcessStartInfo
$psi.FileName = $Executable
$psi.Arguments = "serve"
$psi.UseShellExecute = $false
$psi.RedirectStandardOutput = $true
$psi.RedirectStandardError = $true
$psi.CreateNoWindow = $true
$psi.WorkingDirectory = (Split-Path -Parent $Executable)

$proc = New-Object System.Diagnostics.Process
$proc.StartInfo = $psi
$proc.EnableRaisingEvents = $true

$stdoutWriter = {
  param($sender, $eventArgs)
  if ($null -ne $eventArgs.Data) {
    Write-TimestampedLine -Path $StdoutLog -Line $eventArgs.Data
  }
}
$stderrWriter = {
  param($sender, $eventArgs)
  if ($null -ne $eventArgs.Data) {
    Write-TimestampedLine -Path $StderrLog -Line $eventArgs.Data
  }
}

$null = Register-ObjectEvent -InputObject $proc -EventName OutputDataReceived -Action $stdoutWriter
$null = Register-ObjectEvent -InputObject $proc -EventName ErrorDataReceived -Action $stderrWriter

if (-not $proc.Start()) {
  throw "failed to start ollama serve"
}

$record = [ordered]@{
  pid = $proc.Id
  wrapper_pid = $PID
  port = $Port
  exe = $Executable
  started_at = (Get-Date).ToString("o")
  stdout_log = $StdoutLog
  stderr_log = $StderrLog
}
$record | ConvertTo-Json -Depth 4 | Set-Content -Path $RecordPath -Encoding utf8

$proc.BeginOutputReadLine()
$proc.BeginErrorReadLine()
$proc.WaitForExit()

Write-TimestampedLine -Path $StdoutLog -Line ("process exited code={0}" -f $proc.ExitCode)
exit $proc.ExitCode
