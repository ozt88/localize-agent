param(
  [string]$ProjectDir = "projects/esoteric-ebb/output/batches/canonical_full_retranslate_live",
  [ValidateSet("start","restart","stop","status","cleanup","autoscale","route-no-row","route-overlay-ui","repair-blocked-translate","maintain-failed")]
  [string]$Action = "start",
  [ValidateSet("custom","balanced","score-heavy","retranslate-heavy")]
  [string]$Profile = "custom",
  [int]$TranslateWorkers = 1,
  [int]$FailedTranslateWorkers = 1,
  [int]$ScoreWorkers = 1,
  [int]$RetranslateWorkers = 1,
  [int]$StageBatchSize = 100,
  [int]$LeaseSec = 300,
  [int]$IdleSleepSec = 2,
  [int]$AutoscaleIntervalMinutes = 60,
  [int]$AutoscaleCycles = 0,
  [switch]$CleanupStaleClaims,
  [switch]$Reset,
  [switch]$InitOnly,
  [switch]$Once,
  [switch]$PrintOnly
)

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..\..")
$projectDirAbs = Join-Path $repoRoot $ProjectDir
$timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
$logDir = Join-Path $projectDirAbs ("run_logs\orchestrated_" + $timestamp)
$manifestPath = Join-Path $logDir "workers.json"
$pidDir = Join-Path $projectDirAbs "run_logs\worker_pids"
$binDir = Join-Path $repoRoot "workflow\bin"
$pipelineExe = Join-Path $binDir "go-translation-pipeline.exe"
$projectConfigPath = Join-Path $projectDirAbs "project.json"

$projectDirMatches = New-Object System.Collections.Generic.List[string]
$projectDirMatches.Add($projectDirAbs) | Out-Null
try {
  $projectItem = Get-Item -LiteralPath $projectDirAbs -ErrorAction Stop
  if ($projectItem.Attributes.ToString().Contains("ReparsePoint")) {
    foreach ($targetPath in @($projectItem.Target)) {
      if (-not [string]::IsNullOrWhiteSpace($targetPath)) {
        $resolvedTarget = [System.IO.Path]::GetFullPath($targetPath)
        if (-not $projectDirMatches.Contains($resolvedTarget)) {
          $projectDirMatches.Add($resolvedTarget) | Out-Null
        }
      }
    }
  }
}
catch {
}

function Resolve-ProfileWorkerCounts {
  param(
    [string]$ProfileName,
    [int]$Translate,
    [int]$Score,
    [int]$Retranslate
  )
  switch ($ProfileName) {
    "balanced" {
      return @{
        translate = 1
        failed_translate = 1
        overlay_translate = 1
        score = 3
        retranslate = 2
      }
    }
    "score-heavy" {
      return @{
        translate = 1
        failed_translate = 1
        overlay_translate = 1
        score = 4
        retranslate = 2
      }
    }
    "retranslate-heavy" {
      return @{
        translate = 1
        failed_translate = 1
        overlay_translate = 1
        score = 3
        retranslate = 3
      }
    }
    default {
      return @{
        translate = $Translate
        failed_translate = $FailedTranslateWorkers
        overlay_translate = 1
        score = $Score
        retranslate = $Retranslate
      }
    }
  }
}

function Get-DesiredFailedTranslateWorkers {
  param(
    $Metrics,
    $FailedSummary
  )
  $pendingFailedTranslate = 0
  $translatorNoRow = 0
  if ($null -ne $Metrics -and $null -ne $Metrics.pending_failed_translate) {
    $pendingFailedTranslate = [int]$Metrics.pending_failed_translate
  }
  if ($null -ne $FailedSummary -and $null -ne $FailedSummary.translator_no_row) {
    $translatorNoRow = [int]$FailedSummary.translator_no_row
  }
  $pressure = $pendingFailedTranslate + $translatorNoRow
  if ($pressure -ge 400) {
    return 3
  }
  if ($pressure -ge 100) {
    return 2
  }
  return 1
}

function Get-ProjectWorkerProcesses {
  try {
    $procs = Get-CimInstance Win32_Process -Filter "Name = 'go-translation-pipeline.exe'"
  }
  catch {
    return @()
  }
  return @($procs | Where-Object {
    foreach ($candidate in $projectDirMatches) {
      if ($_.CommandLine -like "*--project-dir $candidate*") {
        return $true
      }
    }
    return $false
  })
}

function New-PipelineArgs {
  param(
    [string]$Role,
    [string]$WorkerId
  )
  $args = @(
    "--project-dir", $projectDirAbs,
    "--worker-role", $Role,
    "--worker-id", $WorkerId,
    "--stage-batch-size", "$StageBatchSize",
    "--lease-sec", "$LeaseSec",
    "--idle-sleep-sec", "$IdleSleepSec"
  )
  if ($Once) {
    $args += "--once"
  }
  return ,$args
}

function Get-WorkerPidPath {
  param([string]$WorkerId)
  return (Join-Path $pidDir ($WorkerId + ".json"))
}

function Read-WorkerPidRecord {
  param([string]$WorkerId)
  $path = Get-WorkerPidPath -WorkerId $WorkerId
  if (-not (Test-Path $path)) {
    return $null
  }
  try {
    return (Get-Content -Path $path -Raw | ConvertFrom-Json)
  }
  catch {
    Remove-Item -Path $path -Force -ErrorAction SilentlyContinue
    return $null
  }
}

function Remove-WorkerPidRecord {
  param([string]$WorkerId)
  $path = Get-WorkerPidPath -WorkerId $WorkerId
  Remove-Item -Path $path -Force -ErrorAction SilentlyContinue
}

function Remove-AllWorkerPidRecords {
  if (-not (Test-Path $pidDir)) {
    return
  }
  Get-ChildItem -Path $pidDir -Filter *.json -ErrorAction SilentlyContinue | Remove-Item -Force -ErrorAction SilentlyContinue
}

function Remove-StaleWorkerPidRecords {
  if (-not (Test-Path $pidDir)) {
    return 0
  }
  $removed = 0
  $pidFiles = Get-ChildItem -Path $pidDir -Filter *.json -ErrorAction SilentlyContinue
  foreach ($file in $pidFiles) {
    try {
      $record = Get-Content -Path $file.FullName -Raw | ConvertFrom-Json
      $workerId = [string]$record.worker_id
      $workerPid = [int]$record.pid
      if (-not (Test-MatchingWorkerProcess -ProcessIdToCheck $workerPid -WorkerId $workerId)) {
        Remove-Item -Path $file.FullName -Force -ErrorAction SilentlyContinue
        $removed++
      }
    }
    catch {
      Remove-Item -Path $file.FullName -Force -ErrorAction SilentlyContinue
      $removed++
    }
  }
  return $removed
}

function Restore-WorkerPidRecordsFromLatestManifest {
  if (-not (Test-Path $projectDirAbs)) {
    return 0
  }
  New-Item -ItemType Directory -Force -Path $pidDir | Out-Null
  $runLogsDir = Join-Path $projectDirAbs "run_logs"
  if (-not (Test-Path $runLogsDir)) {
    return 0
  }
  $manifests = Get-ChildItem -Path $runLogsDir -Directory -ErrorAction SilentlyContinue |
    Where-Object { $_.Name -like "orchestrated_*" } |
    Sort-Object LastWriteTime -Descending
  $restored = 0
  foreach ($dir in $manifests) {
    $manifest = Join-Path $dir.FullName "workers.json"
    if (-not (Test-Path $manifest)) {
      continue
    }
    try {
      $workers = Get-Content -Path $manifest -Raw | ConvertFrom-Json
    }
    catch {
      continue
    }
    foreach ($worker in @($workers)) {
      $workerId = [string]$worker.worker_id
      $workerPid = [int]$worker.pid
      $pidPath = Get-WorkerPidPath -WorkerId $workerId
      if (Test-Path $pidPath) {
        continue
      }
      if (-not (Test-MatchingWorkerProcess -ProcessIdToCheck $workerPid -WorkerId $workerId)) {
        continue
      }
      $record = [ordered]@{
        worker_id = $workerId
        role = [string]$worker.role
        pid = $workerPid
        started_at = ""
        log = [string]$worker.stdout_log
        exe = $pipelineExe
        project_dir = $projectDirAbs
      }
      try {
        $proc = Get-CimInstance Win32_Process -Filter ("ProcessId = {0}" -f $workerPid) -ErrorAction Stop
        if ($null -ne $proc) {
          $record.started_at = [string]$proc.CreationDate
        }
      }
      catch {
      }
      $record | ConvertTo-Json -Depth 4 | Set-Content -Path $pidPath -Encoding utf8
      $restored++
    }
    if ($restored -gt 0) {
      break
    }
  }
  return $restored
}

function Get-WorkerPidRecordStatus {
  if (-not (Test-Path $pidDir)) {
    return @()
  }
  $result = New-Object System.Collections.Generic.List[Object]
  $pidFiles = Get-ChildItem -Path $pidDir -Filter *.json -ErrorAction SilentlyContinue
  foreach ($file in $pidFiles) {
    try {
      $record = Get-Content -Path $file.FullName -Raw | ConvertFrom-Json
      $workerId = [string]$record.worker_id
      $workerPid = [int]$record.pid
      $alive = Test-MatchingWorkerProcess -ProcessIdToCheck $workerPid -WorkerId $workerId
      $result.Add([ordered]@{
        worker_id = $workerId
        pid = $workerPid
        started_at = [string]$record.started_at
        marker = $(if ($alive) { "alive" } else { "stale" })
      }) | Out-Null
    }
    catch {
      $result.Add([ordered]@{
        worker_id = $file.BaseName
        pid = 0
        started_at = ""
        marker = "broken"
      }) | Out-Null
    }
  }
  return $result
}

function Test-MatchingWorkerProcess {
  param(
    [int]$ProcessIdToCheck,
    [string]$WorkerId
  )
  try {
    $proc = Get-CimInstance Win32_Process -Filter "ProcessId = $ProcessIdToCheck"
    if ($null -eq $proc) {
      return $false
    }
    if ($proc.Name -notmatch "go-translation-pipeline(\.exe)?") {
      return $false
    }
    if ($proc.CommandLine -notlike "*--worker-id $WorkerId*") {
      return $false
    }
    foreach ($candidate in $projectDirMatches) {
      if ($proc.CommandLine -like "*--project-dir $candidate*") {
        return $true
      }
    }
    return $false
  }
  catch {
    return $false
  }
}

function Stop-StaleWorkerIfNeeded {
  param([string]$WorkerId)
  $record = Read-WorkerPidRecord -WorkerId $WorkerId
  if ($null -eq $record) {
    return
  }
  $existingPid = 0
  try {
    $existingPid = [int]$record.pid
  }
  catch {
    Remove-WorkerPidRecord -WorkerId $WorkerId
    return
  }
  if ($existingPid -le 0) {
    Remove-WorkerPidRecord -WorkerId $WorkerId
    return
  }
  if (-not (Test-MatchingWorkerProcess -ProcessIdToCheck $existingPid -WorkerId $WorkerId)) {
    Remove-WorkerPidRecord -WorkerId $WorkerId
    return
  }
  try {
    Stop-Process -Id $existingPid -Force -ErrorAction Stop
    Start-Sleep -Milliseconds 300
  }
  catch {
  }
  Remove-WorkerPidRecord -WorkerId $WorkerId
}

function Stop-ProjectWorkers {
  $procs = Get-ProjectWorkerProcesses
  foreach ($proc in $procs) {
    try {
      Stop-Process -Id $proc.ProcessId -Force -ErrorAction Stop
    }
    catch {
    }
  }
  Start-Sleep -Milliseconds 500
  Remove-AllWorkerPidRecords
}

function Write-WorkerPidRecord {
  param(
    [string]$WorkerId,
    [System.Diagnostics.Process]$Process,
    [string]$Role,
    [string]$LogPath
  )
  $record = [ordered]@{
    worker_id = $WorkerId
    role = $Role
    pid = $Process.Id
    started_at = (Get-Date).ToString("o")
    log = $LogPath
    exe = $pipelineExe
    project_dir = $projectDirAbs
  }
  $record | ConvertTo-Json -Depth 4 | Set-Content -Path (Get-WorkerPidPath -WorkerId $WorkerId) -Encoding utf8
}

function Get-ProjectConfigObject {
  if (-not (Test-Path $projectConfigPath)) {
    return $null
  }
  return (Get-Content -Path $projectConfigPath -Raw | ConvertFrom-Json)
}

function Get-ScoreSettings {
  $cfg = Get-ProjectConfigObject
  if ($null -eq $cfg -or $null -eq $cfg.pipeline -or $null -eq $cfg.pipeline.score_llm) {
    return [ordered]@{
      model = ""
      prompt_variant = ""
      batch_size = ""
      concurrency = ""
      backend = ""
      server_url = ""
    }
  }
  return [ordered]@{
    model = [string]$cfg.pipeline.score_llm.model
    prompt_variant = [string]$cfg.pipeline.score_llm.prompt_variant
    batch_size = [string]$cfg.pipeline.score_llm.batch_size
    concurrency = [string]$cfg.pipeline.score_llm.concurrency
    backend = [string]$cfg.pipeline.score_llm.llm_backend
    server_url = [string]$cfg.pipeline.score_llm.server_url
  }
}

function Resolve-CheckpointBackend {
  $cfg = Get-ProjectConfigObject
  if ($null -eq $cfg) {
    return ""
  }
  return [string]$cfg.translation.checkpoint_backend
}

function Resolve-CheckpointTarget {
  $cfg = Get-ProjectConfigObject
  if ($null -eq $cfg) {
    return @{}
  }
  $backend = [string]$cfg.translation.checkpoint_backend
  if ([string]::IsNullOrWhiteSpace($backend)) {
    $backend = "sqlite"
  }
  if ($backend -eq "postgres") {
    return @{
      backend = $backend
      dsn = [string]$cfg.translation.checkpoint_dsn
      db = ""
    }
  }
  $dbPath = [string]$cfg.translation.checkpoint_db
  if (-not [System.IO.Path]::IsPathRooted($dbPath)) {
    $dbPath = Join-Path $projectDirAbs $dbPath
  }
  return @{
    backend = $backend
    dsn = ""
    db = $dbPath
  }
}

function Invoke-PostgresScalar {
  param([string]$Sql)
  $target = Resolve-CheckpointTarget
  $dsn = $target.dsn
  $result = & 'C:\Program Files\PostgreSQL\17\bin\psql.exe' $dsn -At -c $Sql 2>$null
  if ($LASTEXITCODE -ne 0) {
    return ""
  }
  return (($result | Select-Object -First 1) -as [string]).Trim()
}

function Invoke-SQLiteScalar {
  param([string]$Sql)
  $target = Resolve-CheckpointTarget
  $dbPath = $target.db
  $script = @"
import sqlite3
db = r'''$dbPath'''
sql = r'''$Sql'''
conn = sqlite3.connect(db)
try:
    row = conn.execute(sql).fetchone()
    if not row or row[0] is None:
        print("")
    else:
        print(row[0])
finally:
    conn.close()
"@
  return ((($script | python -) | Select-Object -First 1) -as [string]).Trim()
}

function Invoke-CheckpointScalar {
  param([string]$Sql)
  $target = Resolve-CheckpointTarget
  if ($target.backend -eq "postgres") {
    return Invoke-PostgresScalar -Sql $Sql
  }
  return Invoke-SQLiteScalar -Sql $Sql
}

function Invoke-PostgresQuery {
  param([string]$Sql)
  $target = Resolve-CheckpointTarget
  $dsn = $target.dsn
  $tmp = Join-Path $env:TEMP ("pipeline-supervisor-" + [guid]::NewGuid().ToString() + ".sql")
  Set-Content -Path $tmp -Value $Sql -Encoding utf8
  try {
    return (& 'C:\Program Files\PostgreSQL\17\bin\psql.exe' $dsn -f $tmp)
  }
  finally {
    Remove-Item -Path $tmp -Force -ErrorAction SilentlyContinue
  }
}

function Invoke-SQLiteQuery {
  param([string]$Sql)
  $target = Resolve-CheckpointTarget
  $dbPath = $target.db
  $script = @"
import sqlite3
db = r'''$dbPath'''
sql = r'''$Sql'''
conn = sqlite3.connect(db)
try:
    cur = conn.execute(sql)
    cols = [d[0] for d in cur.description] if cur.description else []
    rows = cur.fetchall()
    if cols:
        print(" | ".join(cols))
        for row in rows:
            print(" | ".join("" if v is None else str(v) for v in row))
finally:
    conn.close()
"@
  return ($script | python -)
}

function Invoke-CheckpointQuery {
  param([string]$Sql)
  $target = Resolve-CheckpointTarget
  if ($target.backend -eq "postgres") {
    return Invoke-PostgresQuery -Sql $Sql
  }
  return Invoke-SQLiteQuery -Sql $Sql
}

function Get-LiveWorkerCounts {
  $procs = Get-ProjectWorkerProcesses
  $liveByRole = [ordered]@{
    translate = 0
    failed_translate = 0
    overlay_translate = 0
    score = 0
    retranslate = 0
  }
  foreach ($proc in $procs) {
    if ($proc.CommandLine -like "*--worker-role translate*") { $liveByRole.translate++ }
    elseif ($proc.CommandLine -like "*--worker-role failed-translate*") { $liveByRole.failed_translate++ }
    elseif ($proc.CommandLine -like "*--worker-role overlay-translate*") { $liveByRole.overlay_translate++ }
    elseif ($proc.CommandLine -like "*--worker-role score*") { $liveByRole.score++ }
    elseif ($proc.CommandLine -like "*--worker-role retranslate*") { $liveByRole.retranslate++ }
  }
  return $liveByRole
}

function To-DoubleOrZero {
  param([string]$Value)
  $out = 0.0
  if ([double]::TryParse($Value, [ref]$out)) {
    return $out
  }
  return 0.0
}

function Get-BacklogMetrics {
  $target = Resolve-CheckpointTarget
  if ($target.backend -eq "postgres") {
    $translateRateSql = @"
select round(case when sum(elapsed_ms) > 0 then sum(processed_count)::numeric / (sum(elapsed_ms)::numeric / 1000.0) else 0 end, 3)
from pipeline_worker_stats
where role = 'translate' and finished_at >= now() - interval '20 minutes';
"@
    $scoreRateSql = @"
select round(case when sum(elapsed_ms) > 0 then sum(processed_count)::numeric / (sum(elapsed_ms)::numeric / 1000.0) else 0 end, 3)
from pipeline_worker_stats
where role = 'score' and finished_at >= now() - interval '20 minutes';
"@
    $retranslateRateSql = @"
select round(case when sum(elapsed_ms) > 0 then sum(processed_count)::numeric / (sum(elapsed_ms)::numeric / 1000.0) else 0 end, 3)
from pipeline_worker_stats
where role = 'retranslate' and finished_at >= now() - interval '20 minutes';
"@
  } else {
    $translateRateSql = @"
select round(case when sum(elapsed_ms) > 0 then cast(sum(processed_count) as real) / (cast(sum(elapsed_ms) as real) / 1000.0) else 0 end, 3)
from pipeline_worker_stats
where role = 'translate' and finished_at >= datetime('now','-20 minutes');
"@
    $scoreRateSql = @"
select round(case when sum(elapsed_ms) > 0 then cast(sum(processed_count) as real) / (cast(sum(elapsed_ms) as real) / 1000.0) else 0 end, 3)
from pipeline_worker_stats
where role = 'score' and finished_at >= datetime('now','-20 minutes');
"@
    $retranslateRateSql = @"
select round(case when sum(elapsed_ms) > 0 then cast(sum(processed_count) as real) / (cast(sum(elapsed_ms) as real) / 1000.0) else 0 end, 3)
from pipeline_worker_stats
where role = 'retranslate' and finished_at >= datetime('now','-20 minutes');
"@
  }
  $metrics = [ordered]@{}
  $metrics.pending_translate = [int](Invoke-CheckpointScalar -Sql "select count(*) from pipeline_items where state = 'pending_translate';")
  $metrics.pending_failed_translate = [int](Invoke-CheckpointScalar -Sql "select count(*) from pipeline_items where state = 'pending_failed_translate';")
  $metrics.blocked_translate = [int](Invoke-CheckpointScalar -Sql "select count(*) from pipeline_items where state = 'blocked_translate';")
  $metrics.pending_score = [int](Invoke-CheckpointScalar -Sql "select count(*) from pipeline_items where state in ('pending_score','blocked_score');")
  $metrics.pending_retranslate = [int](Invoke-CheckpointScalar -Sql "select count(*) from pipeline_items where state = 'pending_retranslate';")
  $metrics.translate_rate = To-DoubleOrZero (Invoke-CheckpointScalar -Sql $translateRateSql)
  $metrics.score_rate = To-DoubleOrZero (Invoke-CheckpointScalar -Sql $scoreRateSql)
  $metrics.retranslate_rate = To-DoubleOrZero (Invoke-CheckpointScalar -Sql $retranslateRateSql)
  $metrics.translate_eta_minutes = if ($metrics.translate_rate -gt 0) { [math]::Round((($metrics.pending_translate + $metrics.pending_failed_translate) / $metrics.translate_rate) / 60.0, 1) } else { $null }
  $metrics.score_eta_minutes = if ($metrics.score_rate -gt 0) { [math]::Round(($metrics.pending_score / $metrics.score_rate) / 60.0, 1) } else { $null }
  $metrics.retranslate_eta_minutes = if ($metrics.retranslate_rate -gt 0) { [math]::Round(($metrics.pending_retranslate / $metrics.retranslate_rate) / 60.0, 1) } else { $null }
  return $metrics
}

function Get-ProfileSwitchGuidance {
  return @(
    "balanced: keep when blocked translate dominates and score/retranslate ETA are in a similar range",
    "score-heavy: prefer when pending_score >= 30000 or score ETA is at least 1.5x retranslate ETA",
    "retranslate-heavy: prefer when pending_retranslate >= 5000 or retranslate ETA is at least 1.25x score ETA"
  )
}

function Get-FailedSummary {
  $summary = [ordered]@{
    total = [int](Invoke-CheckpointScalar -Sql "select count(*) from pipeline_items where state = 'failed';")
    translator_no_row = 0
    low_score_max_retry = 0
    missing_score = 0
    top = @()
  }
  $summary.translator_no_row = [int](Invoke-CheckpointScalar -Sql @"
select count(*)
from pipeline_items
where state = 'failed'
  and last_error = 'translator produced no done row';
"@)
  $summary.low_score_max_retry = [int](Invoke-CheckpointScalar -Sql @"
select count(*)
from pipeline_items
where state = 'failed'
  and last_error like 'max score % after max retries';
"@)
  $summary.missing_score = [int](Invoke-CheckpointScalar -Sql @"
select count(*)
from pipeline_items
where state = 'failed'
  and last_error = 'model returned no score for item';
"@)
  $topRows = Invoke-CheckpointQuery -Sql @"
select coalesce(last_error,'') as last_error, count(*) as count
from pipeline_items
where state = 'failed'
group by coalesce(last_error,'')
order by count desc, last_error
limit 5;
"@
  $summary.top = @($topRows)
  return $summary
}

function Get-RecommendedProfile {
  $metrics = Get-BacklogMetrics
  $reason = New-Object System.Collections.Generic.List[string]
  $recommended = "balanced"

  $scoreEta = if ($null -eq $metrics.score_eta_minutes) { 0.0 } else { [double]$metrics.score_eta_minutes }
  $retranslateEta = if ($null -eq $metrics.retranslate_eta_minutes) { 0.0 } else { [double]$metrics.retranslate_eta_minutes }

  if ($metrics.pending_score -ge 30000 -or ($scoreEta -gt 0 -and ($retranslateEta -le 0 -or $scoreEta -ge ($retranslateEta * 1.5)))) {
    $recommended = "score-heavy"
    if ($metrics.pending_score -ge 30000) {
      $reason.Add(("pending_score={0} >= 30000" -f $metrics.pending_score)) | Out-Null
    }
    if ($scoreEta -gt 0 -and ($retranslateEta -le 0 -or $scoreEta -ge ($retranslateEta * 1.5))) {
      $reason.Add(("score_eta={0}m dominates retranslate_eta={1}m" -f $metrics.score_eta_minutes, $metrics.retranslate_eta_minutes)) | Out-Null
    }
  }
  elseif ($metrics.pending_retranslate -ge 5000 -or ($retranslateEta -gt 0 -and ($scoreEta -le 0 -or $retranslateEta -ge ($scoreEta * 1.25)))) {
    $recommended = "retranslate-heavy"
    if ($metrics.pending_retranslate -ge 5000) {
      $reason.Add(("pending_retranslate={0} >= 5000" -f $metrics.pending_retranslate)) | Out-Null
    }
    if ($retranslateEta -gt 0 -and ($scoreEta -le 0 -or $retranslateEta -ge ($scoreEta * 1.25))) {
      $reason.Add(("retranslate_eta={0}m dominates score_eta={1}m" -f $metrics.retranslate_eta_minutes, $metrics.score_eta_minutes)) | Out-Null
    }
  }
  else {
    if ($metrics.blocked_translate -gt ($metrics.pending_translate * 5)) {
      $reason.Add(("blocked_translate={0} dominates pending_translate={1}" -f $metrics.blocked_translate, $metrics.pending_translate)) | Out-Null
    }
    $reason.Add("score/retranslate pressure is within balanced range") | Out-Null
  }

  return [ordered]@{
    profile = $recommended
    reason = ($reason -join "; ")
    metrics = $metrics
  }
}

function Show-SupervisorStatus {
  $counts = Resolve-ProfileWorkerCounts -ProfileName $Profile -Translate $TranslateWorkers -Score $ScoreWorkers -Retranslate $RetranslateWorkers
  $restoredPidRecords = Restore-WorkerPidRecordsFromLatestManifest
  $pidRecordStatus = Get-WorkerPidRecordStatus
  $staleCount = @($pidRecordStatus | Where-Object { $_.marker -eq "stale" -or $_.marker -eq "broken" }).Count
  $scoreSettings = Get-ScoreSettings
  $recommendation = Get-RecommendedProfile
  $failedSummary = Get-FailedSummary
  $counts.failed_translate = Get-DesiredFailedTranslateWorkers -Metrics $recommendation.metrics -FailedSummary $failedSummary
  Write-Output ("ProjectDir: " + $projectDirAbs)
  Write-Output ("Profile: " + $Profile)
  Write-Output ("Desired workers: translate={0} failed-translate={1} overlay-translate={2} score={3} retranslate={4}" -f $counts.translate, $counts.failed_translate, $counts.overlay_translate, $counts.score, $counts.retranslate)
  Write-Output ("PID records restored from manifest: " + $restoredPidRecords)
  Write-Output ("PID record health: total={0} stale_or_broken={1}" -f @($pidRecordStatus).Count, $staleCount)
  Write-Output ("Score settings: model={0} prompt_variant={1} batch={2} concurrency={3} backend={4}" -f $scoreSettings.model, $scoreSettings.prompt_variant, $scoreSettings.batch_size, $scoreSettings.concurrency, $scoreSettings.backend)
  Write-Output ("Recommended profile now: {0}" -f $recommendation.profile)
  Write-Output ("Recommendation reason: {0}" -f $recommendation.reason)
  Write-Output ("Failed summary: total={0} translator_no_row={1} low_score_max_retry={2} missing_score={3}" -f $failedSummary.total, $failedSummary.translator_no_row, $failedSummary.low_score_max_retry, $failedSummary.missing_score)
  if ($failedSummary.top.Count -gt 0) {
    Write-Output "Top failed reasons:"
    foreach ($line in $failedSummary.top) {
      Write-Output ("  " + $line)
    }
  }
  Write-Output "Profile switch conditions:"
  foreach ($line in Get-ProfileSwitchGuidance) {
    Write-Output ("  " + $line)
  }
  Write-Output "Live worker processes:"
  $procs = Get-ProjectWorkerProcesses
  $liveByRole = Get-LiveWorkerCounts
  if ($procs.Count -eq 0) {
    Write-Output "  (none)"
  } else {
    foreach ($proc in $procs | Sort-Object ProcessId) {
      Write-Output ("  pid={0} started={1} cmd={2}" -f $proc.ProcessId, $proc.CreationDate, $proc.CommandLine)
    }
  }
  Write-Output ("Live workers by role: translate={0} failed-translate={1} overlay-translate={2} score={3} retranslate={4}" -f $liveByRole.translate, $liveByRole.failed_translate, $liveByRole.overlay_translate, $liveByRole.score, $liveByRole.retranslate)
  Write-Output "PID records:"
  if (@($pidRecordStatus).Count -eq 0) {
    Write-Output "  (none)"
  } else {
    foreach ($record in $pidRecordStatus | Sort-Object worker_id) {
      Write-Output ("  {0} -> pid={1} started={2} [{3}]" -f $record.worker_id, $record.pid, $record.started_at, $record.marker)
    }
  }
  Write-Output "Pipeline state counts:"
  Invoke-CheckpointQuery -Sql @"
select state, count(*)
from pipeline_items
group by state
order by state;
"@ | ForEach-Object { Write-Output ("  " + $_) }
  Write-Output "Recent worker stats (20m):"
  $target = Resolve-CheckpointTarget
  if ($target.backend -eq "postgres") {
    Invoke-CheckpointQuery -Sql @"
select worker_id, role, sum(processed_count) as processed, sum(elapsed_ms) as elapsed_ms,
       round(case when sum(elapsed_ms) > 0 then sum(processed_count)::numeric / (sum(elapsed_ms)::numeric / 1000.0) else 0 end, 3) as items_per_sec
from pipeline_worker_stats
where finished_at >= now() - interval '20 minutes'
group by worker_id, role
order by worker_id;
"@ | ForEach-Object { Write-Output ("  " + $_) }
    Write-Output "Backlog ETA (20m throughput):"
    Invoke-CheckpointQuery -Sql @"
with perf as (
  select role,
         sum(processed_count)::numeric as processed,
         sum(elapsed_ms)::numeric as elapsed_ms
  from pipeline_worker_stats
  where finished_at >= now() - interval '20 minutes'
  group by role
), rate as (
  select role,
         case when elapsed_ms > 0 then processed / (elapsed_ms / 1000.0) else 0 end as items_per_sec
  from perf
), backlog as (
  select 'translate'::text as role, count(*)::numeric as backlog from pipeline_items where state in ('pending_translate','working_translate')
  union all
  select 'failed-translate'::text as role, count(*)::numeric as backlog from pipeline_items where state in ('pending_failed_translate','working_failed_translate')
  union all
  select 'overlay-translate'::text as role, count(*)::numeric as backlog from pipeline_items where state in ('pending_overlay_translate','working_overlay_translate')
  union all
  select 'score'::text as role, count(*)::numeric as backlog from pipeline_items where state in ('pending_score','working_score','blocked_score')
  union all
  select 'retranslate'::text as role, count(*)::numeric as backlog from pipeline_items where state in ('pending_retranslate','working_retranslate')
)
select b.role,
       b.backlog,
       round(coalesce(r.items_per_sec,0), 3) as items_per_sec,
       case when coalesce(r.items_per_sec,0) > 0 then round((b.backlog / r.items_per_sec) / 60.0, 1) else null end as eta_minutes
from backlog b
left join rate r on r.role = b.role
order by b.role;
"@ | ForEach-Object { Write-Output ("  " + $_) }
  } else {
    Invoke-CheckpointQuery -Sql @"
select worker_id, role, sum(processed_count) as processed, sum(elapsed_ms) as elapsed_ms
from pipeline_worker_stats
group by worker_id, role
order by worker_id;
"@ | ForEach-Object { Write-Output ("  " + $_) }
  }
}

function Invoke-SupervisorAction {
  param(
    [string]$SupervisorAction,
    [string]$SupervisorProfile
  )
  $scriptPath = Join-Path $repoRoot "projects\esoteric-ebb\cmd\run_pipeline_orchestrated.ps1"
  $args = @(
    "-NoProfile",
    "-ExecutionPolicy", "Bypass",
    "-File", $scriptPath,
    "-ProjectDir", $ProjectDir,
    "-Action", $SupervisorAction,
    "-Profile", $SupervisorProfile,
    "-StageBatchSize", "$StageBatchSize",
    "-LeaseSec", "$LeaseSec",
    "-IdleSleepSec", "$IdleSleepSec"
  )
  & powershell @args
  return $LASTEXITCODE
}

Push-Location $repoRoot
try {
  $resolvedCounts = Resolve-ProfileWorkerCounts -ProfileName $Profile -Translate $TranslateWorkers -Score $ScoreWorkers -Retranslate $RetranslateWorkers
  $startupMetrics = Get-BacklogMetrics
  $startupFailedSummary = Get-FailedSummary
  $resolvedCounts.failed_translate = Get-DesiredFailedTranslateWorkers -Metrics $startupMetrics -FailedSummary $startupFailedSummary
  $TranslateWorkers = [int]$resolvedCounts.translate
  $FailedTranslateWorkers = [int]$resolvedCounts.failed_translate
  $ScoreWorkers = [int]$resolvedCounts.score
  $RetranslateWorkers = [int]$resolvedCounts.retranslate

  switch ($Action) {
    "status" {
      Show-SupervisorStatus
      exit 0
    }
    "stop" {
      Stop-ProjectWorkers
      Write-Output "Stopped project workers."
      exit 0
    }
    "cleanup" {
      $removedPidRecords = Remove-StaleWorkerPidRecords
      Write-Output ("Removed stale PID records: " + $removedPidRecords)
      $CleanupStaleClaims = $true
    }
    "restart" {
      Stop-ProjectWorkers
    }
    "autoscale" {
      $cycles = 0
      while ($true) {
        $recommendation = Get-RecommendedProfile
        $failedSummary = Get-FailedSummary
        $recommendedProfile = [string]$recommendation.profile
        $liveByRole = Get-LiveWorkerCounts
        $desiredByRecommendation = Resolve-ProfileWorkerCounts -ProfileName $recommendedProfile -Translate $TranslateWorkers -Score $ScoreWorkers -Retranslate $RetranslateWorkers
        $desiredByRecommendation.failed_translate = Get-DesiredFailedTranslateWorkers -Metrics $recommendation.metrics -FailedSummary $failedSummary
        $matchesLive = ($liveByRole.translate -eq $desiredByRecommendation.translate) -and ($liveByRole.failed_translate -eq $desiredByRecommendation.failed_translate) -and ($liveByRole.overlay_translate -eq $desiredByRecommendation.overlay_translate) -and ($liveByRole.score -eq $desiredByRecommendation.score) -and ($liveByRole.retranslate -eq $desiredByRecommendation.retranslate)
        Write-Output ("[{0}] autoscale recommendation={1} reason={2}" -f (Get-Date).ToString("o"), $recommendedProfile, $recommendation.reason)
        Write-Output ("[{0}] failed total={1} translator_no_row={2} low_score_max_retry={3} missing_score={4}" -f (Get-Date).ToString("o"), $failedSummary.total, $failedSummary.translator_no_row, $failedSummary.low_score_max_retry, $failedSummary.missing_score)
        if (-not $matchesLive) {
          Write-Output ("[{0}] restarting to profile {1}" -f (Get-Date).ToString("o"), $recommendedProfile)
          if ($PrintOnly) {
            Write-Output ("powershell -NoProfile -ExecutionPolicy Bypass -File {0} -ProjectDir {1} -Action restart -Profile {2}" -f (Join-Path $repoRoot "projects\esoteric-ebb\cmd\run_pipeline_orchestrated.ps1"), $ProjectDir, $recommendedProfile)
          } else {
            $restartExit = Invoke-SupervisorAction -SupervisorAction "restart" -SupervisorProfile $recommendedProfile
            if ($restartExit -ne 0) {
              throw "autoscale restart failed with exit code $restartExit"
            }
          }
        } else {
          Write-Output ("[{0}] live worker counts already match recommended profile {1}" -f (Get-Date).ToString("o"), $recommendedProfile)
        }

        if ($failedSummary.translator_no_row -gt 0) {
          Write-Output ("[{0}] running failed maintenance for translator_no_row={1}" -f (Get-Date).ToString("o"), $failedSummary.translator_no_row)
          if ($PrintOnly) {
            Write-Output ("powershell -NoProfile -ExecutionPolicy Bypass -File {0} -ProjectDir {1} -Action maintain-failed -Profile {2}" -f (Join-Path $repoRoot "projects\esoteric-ebb\cmd\run_pipeline_orchestrated.ps1"), $ProjectDir, $recommendedProfile)
          } else {
            $maintainExit = Invoke-SupervisorAction -SupervisorAction "maintain-failed" -SupervisorProfile $recommendedProfile
            if ($maintainExit -ne 0) {
              throw "failed maintenance failed with exit code $maintainExit"
            }
          }
        }

        if ($AutoscaleCycles -gt 0) {
          $cycles++
          if ($cycles -ge $AutoscaleCycles) {
            exit 0
          }
        }
        if ($PrintOnly) {
          exit 0
        }
        Start-Sleep -Seconds ([math]::Max(60, $AutoscaleIntervalMinutes * 60))
      }
    }
  }

  if (-not $PrintOnly) {
    New-Item -ItemType Directory -Force -Path $binDir | Out-Null
    New-Item -ItemType Directory -Force -Path $pidDir | Out-Null
    & go build -o $pipelineExe ./workflow/cmd/go-translation-pipeline
    if ($LASTEXITCODE -ne 0) {
      throw "pipeline binary build failed with exit code $LASTEXITCODE"
    }
  }

  $initArgs = @(
    "--project-dir", $projectDirAbs,
    "--stage-batch-size", "$StageBatchSize",
    "--init-only"
  )
  if ($Reset) {
    $initArgs += "--reset"
  }

  if ($PrintOnly) {
    Write-Output ("build -> " + $pipelineExe)
    if ($CleanupStaleClaims) {
      Write-Output ($pipelineExe + " " + (@("--project-dir", $projectDirAbs, "--cleanup-stale-claims") -join " "))
      exit 0
    }
    if ($Action -eq "route-no-row") {
      Write-Output ($pipelineExe + " " + (@("--project-dir", $projectDirAbs, "--route-known-failed-no-row") -join " "))
      exit 0
    }
    if ($Action -eq "route-overlay-ui") {
      Write-Output ($pipelineExe + " " + (@("--project-dir", $projectDirAbs, "--route-overlay-ui") -join " "))
      exit 0
    }
    if ($Action -eq "repair-blocked-translate") {
      Write-Output ($pipelineExe + " " + (@("--project-dir", $projectDirAbs, "--repair-blocked-translate") -join " "))
      exit 0
    }
    if ($Action -eq "maintain-failed") {
      Write-Output ($pipelineExe + " " + (@("--project-dir", $projectDirAbs, "--route-known-failed-no-row") -join " "))
      Write-Output ($pipelineExe + " " + (@("--project-dir", $projectDirAbs, "--repair-blocked-translate") -join " "))
      exit 0
    }
    Write-Output ($pipelineExe + " " + ($initArgs -join " "))
    foreach ($spec in @(
      @{ Role = "translate"; Count = $TranslateWorkers },
      @{ Role = "failed-translate"; Count = $FailedTranslateWorkers },
      @{ Role = "overlay-translate"; Count = $resolvedCounts.overlay_translate },
      @{ Role = "score"; Count = $ScoreWorkers },
      @{ Role = "retranslate"; Count = $RetranslateWorkers }
    )) {
      for ($i = 1; $i -le $spec.Count; $i++) {
        $workerId = "$($spec.Role)-$i"
        Write-Output ($pipelineExe + " " + ((New-PipelineArgs -Role $spec.Role -WorkerId $workerId) -join " "))
      }
    }
    exit 0
  }

  New-Item -ItemType Directory -Force -Path $logDir | Out-Null

  if ($CleanupStaleClaims) {
    & $pipelineExe --project-dir $projectDirAbs --cleanup-stale-claims
    if ($LASTEXITCODE -ne 0) {
      throw "pipeline stale-claim cleanup failed with exit code $LASTEXITCODE"
    }
    exit 0
  }
  if ($Action -eq "repair-blocked-translate") {
    & $pipelineExe --project-dir $projectDirAbs --repair-blocked-translate
    if ($LASTEXITCODE -ne 0) {
      throw "pipeline blocked-translate repair failed with exit code $LASTEXITCODE"
    }
    exit 0
  }
  if ($Action -eq "maintain-failed") {
    & $pipelineExe --project-dir $projectDirAbs --route-known-failed-no-row
    if ($LASTEXITCODE -ne 0) {
      throw "pipeline failed-router execution failed with exit code $LASTEXITCODE"
    }
    & $pipelineExe --project-dir $projectDirAbs --repair-blocked-translate
    if ($LASTEXITCODE -ne 0) {
      throw "pipeline blocked-translate repair failed with exit code $LASTEXITCODE"
    }
    exit 0
  }
  if ($Action -eq "route-no-row") {
    & $pipelineExe --project-dir $projectDirAbs --route-known-failed-no-row
    if ($LASTEXITCODE -ne 0) {
      throw "pipeline failed-router execution failed with exit code $LASTEXITCODE"
    }
    exit 0
  }
  if ($Action -eq "route-overlay-ui") {
    & $pipelineExe --project-dir $projectDirAbs --route-overlay-ui
    if ($LASTEXITCODE -ne 0) {
      throw "pipeline overlay route execution failed with exit code $LASTEXITCODE"
    }
    exit 0
  }

  & $pipelineExe @initArgs
  if ($LASTEXITCODE -ne 0) {
    throw "pipeline init failed with exit code $LASTEXITCODE"
  }
  if ($InitOnly) {
    exit 0
  }

  $workers = New-Object System.Collections.Generic.List[Object]
  foreach ($spec in @(
      @{ Role = "translate"; Count = $TranslateWorkers },
      @{ Role = "failed-translate"; Count = $FailedTranslateWorkers },
      @{ Role = "overlay-translate"; Count = $resolvedCounts.overlay_translate },
      @{ Role = "score"; Count = $ScoreWorkers },
      @{ Role = "retranslate"; Count = $RetranslateWorkers }
  )) {
    for ($i = 1; $i -le $spec.Count; $i++) {
      $workerId = "$($spec.Role)-$i"
      Stop-StaleWorkerIfNeeded -WorkerId $workerId
      $roleArgs = New-PipelineArgs -Role $spec.Role -WorkerId $workerId
      $stdoutPath = Join-Path $logDir ($workerId + ".stdout.log")
      $stderrPath = Join-Path $logDir ($workerId + ".stderr.log")
      $proc = Start-Process -FilePath $pipelineExe -ArgumentList $roleArgs -RedirectStandardOutput $stdoutPath -RedirectStandardError $stderrPath -WindowStyle Hidden -PassThru -WorkingDirectory $repoRoot
      Write-WorkerPidRecord -WorkerId $workerId -Process $proc -Role $spec.Role -LogPath $stdoutPath
      $workers.Add([ordered]@{
        role = $spec.Role
        worker_id = $workerId
        pid = $proc.Id
        stdout_log = $stdoutPath
        stderr_log = $stderrPath
        exe = $pipelineExe
        pid_file = (Get-WorkerPidPath -WorkerId $workerId)
      }) | Out-Null
    }
  }

  $workers | ConvertTo-Json -Depth 4 | Set-Content -Path $manifestPath -Encoding utf8
  Write-Output "Started workers:"
  $workers | ForEach-Object {
    Write-Output ("  [{0}] {1} pid={2} stdout={3}" -f $_.role, $_.worker_id, $_.pid, $_.stdout_log)
  }
  Write-Output "Running failed maintenance bootstrap..."
  & $pipelineExe --project-dir $projectDirAbs --route-known-failed-no-row
  if ($LASTEXITCODE -ne 0) {
    throw "pipeline failed-router bootstrap failed with exit code $LASTEXITCODE"
  }
  & $pipelineExe --project-dir $projectDirAbs --repair-blocked-translate
  if ($LASTEXITCODE -ne 0) {
    throw "pipeline blocked-translate bootstrap repair failed with exit code $LASTEXITCODE"
  }
  Write-Output "Manifest: $manifestPath"
}
finally {
  Pop-Location
}
