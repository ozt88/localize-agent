param(
  [int]$Iterations = 5,
  [int]$WaveSize = 20,
  [int]$BatchSize = 5,
  [int]$TimeoutSec = 300
)

$ErrorActionPreference = "Stop"
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot

$dictionary = "projects/esoteric-ebb/patch/output/source_overlay_analysis/esoteric_ebb_glossary_dictionary_20260316.json"
$preserve = "projects/esoteric-ebb/patch/output/source_overlay_analysis/esoteric_ebb_glossary_preserve_terms_20260316.json"
$artifact = "projects/esoteric-ebb/patch/output/source_overlay_analysis/esoteric_ebb_glossary_autosuggested_20260316.json"
$progress = "workflow/output/glossary_autosuggest_progress_20260316.log"
$loopLog = "workflow/output/glossary_autosuggest_loop_20260316.log"

function Get-TranslateCount {
  if (-not (Test-Path $artifact)) { return 0 }
  $script = @'
import json, pathlib
p = pathlib.Path(r"projects/esoteric-ebb/patch/output/source_overlay_analysis/esoteric_ebb_glossary_autosuggested_20260316.json")
data = json.loads(p.read_text(encoding="utf-8"))
print(len(data.get("translate_terms", [])))
'@
  return [int](@($script | python -)[0])
}

$before = Get-TranslateCount
for ($i = 1; $i -le $Iterations; $i++) {
  Add-Content -Path $loopLog -Value ("[{0}] wave {1} start before={2}" -f (Get-Date -Format s), $i, $before)
  python scripts/generate_glossary_autosuggest.py `
    --dictionary $dictionary `
    --preserve $preserve `
    --base-curated $artifact `
    --out $artifact `
    --progress-log $progress `
    --batch-size $BatchSize `
    --max-items $WaveSize `
    --timeout-sec $TimeoutSec
  $after = Get-TranslateCount
  Add-Content -Path $loopLog -Value ("[{0}] wave {1} done after={2}" -f (Get-Date -Format s), $i, $after)
  if ($after -le $before) {
    Add-Content -Path $loopLog -Value ("[{0}] wave {1} no growth; stopping" -f (Get-Date -Format s), $i)
    break
  }
  $before = $after
}
