param(
  [string]$LogPath = "c:\Users\DELL\Desktop\localize-agent\workflow\output\go_translation_pipeline_process_watch.log",
  [int]$IntervalSec = 1
)

$ErrorActionPreference = "Stop"

function Write-ProcessSnapshot {
  param(
    [string]$LogPath
  )

  $timestamp = (Get-Date).ToUniversalTime().ToString("o")
  $rows = Get-CimInstance Win32_Process | Where-Object {
    $_.Name -eq 'go-translation-pipeline.exe' -or
    ($_.Name -eq 'go.exe' -and $_.CommandLine -like '*go-translation-pipeline*') -or
    ($_.Name -eq 'powershell.exe' -and $_.CommandLine -like '*go-translation-pipeline*')
  } | Sort-Object ProcessId

  Add-Content -Path $LogPath -Value ("=== " + $timestamp + " ===")
  if (-not $rows) {
    Add-Content -Path $LogPath -Value "(none)"
    return
  }

  foreach ($row in $rows) {
    $line = [ordered]@{
      timestamp = $timestamp
      pid = $row.ProcessId
      ppid = $row.ParentProcessId
      name = $row.Name
      command_line = $row.CommandLine
    } | ConvertTo-Json -Compress
    Add-Content -Path $LogPath -Value $line
  }
}

New-Item -ItemType Directory -Force -Path ([System.IO.Path]::GetDirectoryName($LogPath)) | Out-Null
Write-ProcessSnapshot -LogPath $LogPath

while ($true) {
  Start-Sleep -Seconds $IntervalSec
  Write-ProcessSnapshot -LogPath $LogPath
}
