$binDir = "projects/esoteric-ebb/.bin"
$exePath = Join-Path $binDir "go-translate.exe"

New-Item -ItemType Directory -Force -Path $binDir | Out-Null
go build -o $exePath ./workflow/cmd/go-translate
