param(
  [string]$ProjectDir = "projects/esoteric-ebb/output/batches/canonical_full_retranslate_live",
  [string]$WorkerRole = "all",
  [int]$StageBatchSize = 100,
  [switch]$Reset,
  [switch]$InitOnly,
  [switch]$Once,
  [switch]$PrintOnly
)

$cmd = @(
  "go", "run", "./workflow/cmd/go-translation-pipeline",
  "--project-dir", $ProjectDir,
  "--worker-role", $WorkerRole,
  "--stage-batch-size", "$StageBatchSize"
)

if ($Reset) {
  $cmd += "--reset"
}
if ($InitOnly) {
  $cmd += "--init-only"
}
if ($Once) {
  $cmd += "--once"
}

if ($PrintOnly) {
  $cmd -join " "
  exit 0
}

& $cmd[0] $cmd[1..($cmd.Length-1)]
