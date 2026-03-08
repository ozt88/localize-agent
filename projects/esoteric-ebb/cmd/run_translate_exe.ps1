$exePath = "projects/esoteric-ebb/.bin/go-translate.exe"

if (-not (Test-Path $exePath)) {
  Write-Error "missing exe: $exePath"
  exit 1
}

$idsFile = "projects/esoteric-ebb/source/prepared/ids_esoteric.txt"
$checkpointDb = ""
$traceOut = ""

if ($args.Length -ge 1 -and $args[0]) {
  $idsFile = $args[0]
}
if ($args.Length -ge 2 -and $args[1]) {
  $checkpointDb = $args[1]
}
if ($args.Length -ge 3 -and $args[2]) {
  $traceOut = $args[2]
}

$cmd = @($exePath, "--project", "esoteric-ebb", "--ids-file", $idsFile)
if ($checkpointDb -ne "") {
  $cmd += @("--checkpoint-db", $checkpointDb)
}
if ($traceOut -ne "") {
  $cmd += @("--trace-out", $traceOut)
}

& $cmd[0] $cmd[1..($cmd.Length-1)]
