param(
  [string]$BatchName = ("canonical_full_retranslate_dual_score_" + (Get-Date -Format "yyyyMMdd_HHmmss")),
  [string]$Source = "projects/esoteric-ebb/output/batches/canonical_full_retranslate/canonical_translation_source.json",
  [string]$Reference = "projects/esoteric-ebb/output/batches/canonical_full_retranslate/canonical_translation_reference_ko.json",
  [string]$ServerUrl = "http://127.0.0.1:4112",
  [string]$TranslateModel = "openai/gpt-5.4",
  [string]$JudgeModel = "openai/gpt-5.2",
  [string]$RewriteModel = "openai/gpt-5.4",
  [int]$StageBatchSize = 100,
  [double]$RewriteThreshold = 70,
  [int]$MaxRetries = 3,
  [int]$TranslateConcurrency = 4,
  [int]$TranslateBatchSize = 8,
  [int]$TranslateTimeoutSec = 120,
  [int]$JudgeConcurrency = 4,
  [int]$JudgeBatchSize = 20,
  [int]$JudgeTimeoutSec = 120,
  [int]$RewriteConcurrency = 2,
  [int]$RewriteBatchSize = 10,
  [int]$RewriteTimeoutSec = 120,
  [switch]$PrintOnly
)

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..\..")
$outDir = Join-Path $repoRoot ("projects\esoteric-ebb\output\batches\" + $BatchName)
$sourcePath = Join-Path $repoRoot $Source
$referencePath = Join-Path $repoRoot $Reference

$buildCmd = @(
  "python",
  "projects/esoteric-ebb/patch/tools/build_canonical_translation_batch.py",
  "--source", $sourcePath,
  "--reference", $referencePath,
  "--out-dir", $outDir
)

if ($PrintOnly) {
  $buildCmd -join " "
  exit 0
}

Push-Location $repoRoot
try {
  & $buildCmd[0] $buildCmd[1..($buildCmd.Length-1)]
  if ($LASTEXITCODE -ne 0) {
    throw "batch builder failed with exit code $LASTEXITCODE"
  }

  $project = [ordered]@{
    name = "esoteric-ebb-$BatchName"
    translation = [ordered]@{
      source = "./source_esoteric.json"
      current = "./current_esoteric.json"
      ids_file = "./ids_esoteric.txt"
      translator_package_chunks = "./canonical_translation_chunks.json"
      checkpoint_db = "./translation_checkpoint.db"
      context_files = @(
        "../../../../context/esoteric_ebb_modelfile_system.md"
      )
      rules_file = ""
      server_url = $ServerUrl
      model = $TranslateModel
      llm_backend = "opencode"
      translator_response_mode = "plain"
    }
    evaluation = [ordered]@{
      pack_in = "./eval_pack.json"
      db = "./evaluation_unified.db"
      run_name = $BatchName
      context_files = @(
        "../../../../context/esoteric_ebb_modelfile_system.md"
      )
      rules_file = "../../../../context/esoteric_ebb_rules.md"
      eval_rules_file = "../../../../context/esoteric_ebb_rules.md"
      server_url = "http://127.0.0.1:11434"
      trans_model = "TranslateGemma:latest"
      eval_model = "TranslateGemma:latest"
      llm_backend = "ollama"
    }
    pipeline = [ordered]@{
      stage_batch_size = $StageBatchSize
      threshold = $RewriteThreshold
      max_retries = $MaxRetries
      low_llm = [ordered]@{
        llm_backend = "opencode"
        server_url = $ServerUrl
        model = $TranslateModel
        translator_response_mode = "plain"
        concurrency = $TranslateConcurrency
        batch_size = $TranslateBatchSize
        timeout_sec = $TranslateTimeoutSec
      }
      high_llm = [ordered]@{
        llm_backend = "opencode"
        server_url = $ServerUrl
        model = $RewriteModel
        concurrency = $RewriteConcurrency
        batch_size = $RewriteBatchSize
        timeout_sec = $RewriteTimeoutSec
      }
      score_llm = [ordered]@{
        llm_backend = "opencode"
        server_url = $ServerUrl
        model = $JudgeModel
        concurrency = $JudgeConcurrency
        batch_size = $JudgeBatchSize
        timeout_sec = $JudgeTimeoutSec
      }
    }
  }

  $projectPath = Join-Path $outDir "project.json"
  $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
  [System.IO.File]::WriteAllText($projectPath, ($project | ConvertTo-Json -Depth 10), $utf8NoBom)

  Write-Output ""
  Write-Output "Created batch:"
  Write-Output "  ProjectDir: $outDir"
  Write-Output "  Project:    $projectPath"
  Write-Output "  Checkpoint: $(Join-Path $outDir 'translation_checkpoint.db')"
}
finally {
  Pop-Location
}
