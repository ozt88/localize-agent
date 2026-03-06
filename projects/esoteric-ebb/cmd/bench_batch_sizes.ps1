param(
  [string]$SourceIDsFile = "projects/esoteric-ebb/output/batches/translation_assetripper_textasset_unique/ids_esoteric.txt",
  [string]$Model = "TranslateGemma:latest",
  [int[]]$BatchSizes = @(4, 6, 8),
  [int]$SampleCount = 80
)

$IDsFile = "projects/esoteric-ebb/output/bench_ids_tmp_$SampleCount.txt"
Get-Content $SourceIDsFile -TotalCount $SampleCount | Set-Content $IDsFile

foreach ($BatchSize in $BatchSizes) {
  $DbPath = "projects/esoteric-ebb/output/bench_batch_${BatchSize}.db"
  Write-Host "Running batch-size=$BatchSize ids=$IDsFile db=$DbPath"
  go run ./workflow/cmd/go-translate `
    --project esoteric-ebb `
    --model $Model `
    --ids-file $IDsFile `
    --checkpoint-db $DbPath `
    --concurrency 1 `
    --batch-size $BatchSize
}
