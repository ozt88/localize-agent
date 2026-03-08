$log = Join-Path $PSScriptRoot "..\output\batches\translation_assetripper_textasset_unique\ollama_11435.log"
$env:OLLAMA_HOST = "127.0.0.1:11435"
$env:OLLAMA_NUM_PARALLEL = "2"
$env:OLLAMA_MAX_LOADED_MODELS = "1"
$env:OLLAMA_KEEP_ALIVE = "30m"
ollama serve *>> $log
