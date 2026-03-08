$projectRoot = "projects/esoteric-ebb"
$canonicalSystemPromptPath = Join-Path $projectRoot "context/esoteric_ebb_modelfile_system.md"
$modelfilePath = Join-Path $projectRoot "ollama/TranslateGemma-fast.Modelfile"

if (-not (Test-Path $canonicalSystemPromptPath)) {
  Write-Error "missing system prompt file: $canonicalSystemPromptPath"
  exit 1
}

$systemPrompt = Get-Content -Raw $canonicalSystemPromptPath
$modelfile = @"
FROM TranslateGemma:latest

PARAMETER temperature 0
PARAMETER num_ctx 8192

SYSTEM """
$systemPrompt
"""
"@

Set-Content -Path $modelfilePath -Value $modelfile -Encoding UTF8
ollama create TranslateGemma-fast:latest -f $modelfilePath
