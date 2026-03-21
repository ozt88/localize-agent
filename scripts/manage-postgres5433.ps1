param(
  [ValidateSet("start", "stop", "restart", "status")]
  [string]$Action = "status",
  [int]$Port = 5433,
  [string]$DataDir = "c:\Users\DELL\Desktop\localize-agent\workflow\output\postgres17_data",
  [string]$PgCtl = "C:\Program Files\PostgreSQL\17\bin\pg_ctl.exe",
  [string]$Psql = "C:\Program Files\PostgreSQL\17\bin\psql.exe",
  [string]$DBDsn = "postgres://postgres@127.0.0.1:5433/localize_agent?sslmode=disable",
  [string]$LogPath = "c:\Users\DELL\Desktop\localize-agent\workflow\output\postgres17.log"
)

function Test-ListeningPort {
  param([int]$ListenPort)
  $conn = Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue | Where-Object { $_.LocalPort -eq $ListenPort } | Select-Object -First 1
  return $null -ne $conn
}

function Test-DBReady {
  param([string]$Dsn, [string]$PsqlPath)
  if (-not (Test-Path $PsqlPath)) {
    return $false
  }
  try {
    $result = & $PsqlPath $Dsn -At -c "select count(*) from pg_database where datname='localize_agent';" 2>$null
    return ($LASTEXITCODE -eq 0) -and ([string]::Join("", $result).Trim() -eq "1")
  }
  catch {
    return $false
  }
}

function Show-Status {
  $listening = Test-ListeningPort -ListenPort $Port
  $ready = $false
  if ($listening) {
    $ready = Test-DBReady -Dsn $DBDsn -PsqlPath $Psql
  }
  $status = if ($listening -and $ready) { "running" } elseif ($listening) { "port_only" } else { "stopped" }
  Write-Output ("status=" + $status)
  Write-Output ("port=" + $Port)
  Write-Output ("data_dir=" + $DataDir)
  Write-Output ("port_listen=" + ($(if ($listening) { "true" } else { "false" })))
  Write-Output ("db_ready=" + ($(if ($ready) { "true" } else { "false" })))
}

if (-not (Test-Path $PgCtl)) {
  throw "pg_ctl.exe not found: $PgCtl"
}
if (-not (Test-Path $DataDir)) {
  throw "postgres data dir not found: $DataDir"
}

switch ($Action) {
  "status" {
    Show-Status
    exit 0
  }
  "stop" {
    & $PgCtl -D $DataDir -m fast -w stop | Out-Null
    Show-Status
    exit 0
  }
  "restart" {
    & $PgCtl -D $DataDir -m fast -w stop | Out-Null
    $Action = "start"
  }
}

if (-not (Test-ListeningPort -ListenPort $Port)) {
  & $PgCtl -D $DataDir -l $LogPath -o (" -p {0} " -f $Port) start | Out-Null
}

for ($i = 0; $i -lt 20; $i++) {
  Start-Sleep -Milliseconds 500
  if ((Test-ListeningPort -ListenPort $Port) -and (Test-DBReady -Dsn $DBDsn -PsqlPath $Psql)) {
    break
  }
}

Show-Status

