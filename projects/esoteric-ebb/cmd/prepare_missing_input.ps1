$inPath = "projects/esoteric-ebb/source/retry/retry_pipeline_package.json"
$outDir = "projects/esoteric-ebb/output/batches/missing_translate"

if ($args.Length -ge 1 -and $args[0]) {
  $inPath = $args[0]
}
if ($args.Length -ge 2 -and $args[1]) {
  $outDir = $args[1]
}

go run ./workflow/cmd/go-esoteric-adapt-in `
  --in $inPath `
  --out-dir $outDir
