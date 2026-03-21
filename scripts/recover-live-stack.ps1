param(
  [ValidateSet("status", "recover")]
  [string]$Action = "recover",
  [ValidateSet("custom", "balanced", "score-heavy", "retranslate-heavy")]
  [string]$Profile = "custom"
)

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$postgresScript = Join-Path $repoRoot "scripts\manage-postgres5433.ps1"
$opencodeScript = Join-Path $repoRoot "scripts\manage-opencode-serve.ps1"
$reviewScript = Join-Path $repoRoot "scripts\manage-review.ps1"
$pipelineScript = Join-Path $repoRoot "projects\esoteric-ebb\cmd\run_pipeline_orchestrated.ps1"

function Invoke-Step {
  param([string]$Label, [scriptblock]$Call)
  Write-Output ("== " + $Label + " ==")
  & $Call
}

if ($Action -eq "status") {
  Invoke-Step -Label "postgres5433" -Call { & $postgresScript -Action status }
  Invoke-Step -Label "opencode" -Call { & $opencodeScript -Action status }
  Invoke-Step -Label "review" -Call { & $reviewScript -Action status }
  Invoke-Step -Label "pipeline" -Call { & $pipelineScript -Action status -Profile $Profile }
  exit 0
}

Invoke-Step -Label "postgres5433 restart" -Call { & $postgresScript -Action restart }
Invoke-Step -Label "opencode restart" -Call { & $opencodeScript -Action restart }
Invoke-Step -Label "review restart" -Call { & $reviewScript -Action restart }
Invoke-Step -Label "pipeline restart" -Call { & $pipelineScript -Action restart -Profile $Profile }
Invoke-Step -Label "final status" -Call { & $pipelineScript -Action status -Profile $Profile }

