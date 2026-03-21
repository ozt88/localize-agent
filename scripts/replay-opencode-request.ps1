param(
  [Parameter(Mandatory = $true)]
  [string]$ServerUrl,
  [Parameter(Mandatory = $true)]
  [string]$Path,
  [string]$BodyFile,
  [string]$BodyBase64File,
  [string]$Directory = "c:\Users\DELL\Desktop\localize-agent",
  [string]$OutFile = ""
)

if ([string]::IsNullOrWhiteSpace($BodyFile) -and [string]::IsNullOrWhiteSpace($BodyBase64File)) {
  throw "Either -BodyFile or -BodyBase64File is required."
}

$query = "?directory=" + [uri]::EscapeDataString($Directory)
$url = ($ServerUrl.TrimEnd('/')) + $Path + $query
$bodyBytes = $null
if (-not [string]::IsNullOrWhiteSpace($BodyBase64File)) {
  $b64 = Get-Content -Raw -Encoding UTF8 $BodyBase64File
  $bodyBytes = [Convert]::FromBase64String($b64.Trim())
} else {
  $body = Get-Content -Raw -Encoding UTF8 $BodyFile
  $bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($body)
}
$body = [System.Text.Encoding]::UTF8.GetString($bodyBytes)

$result = [ordered]@{
  timestamp = (Get-Date).ToUniversalTime().ToString("o")
  url = $url
  body_bytes = $bodyBytes.Length
  ok = $false
  status = $null
  response_bytes = $null
  response_raw = $null
  error = $null
}

try {
  $resp = Invoke-WebRequest -Uri $url -Method Post -ContentType "application/json" -Body $bodyBytes -TimeoutSec 120 -ErrorAction Stop
  $result.ok = $true
  $result.status = [int]$resp.StatusCode
  $result.response_raw = [string]$resp.Content
  $result.response_bytes = [System.Text.Encoding]::UTF8.GetByteCount([string]$resp.Content)
}
catch {
  $response = $_.Exception.Response
  if ($response -ne $null) {
    try {
      $stream = $response.GetResponseStream()
      if ($stream -ne $null) {
        $reader = New-Object System.IO.StreamReader($stream)
        $raw = $reader.ReadToEnd()
        $result.response_raw = $raw
        $result.response_bytes = [System.Text.Encoding]::UTF8.GetByteCount([string]$raw)
      }
      $result.status = [int]$response.StatusCode
    }
    catch {
    }
  }
  $result.error = $_.Exception.Message
}

$json = $result | ConvertTo-Json -Depth 6
if ($OutFile -ne "") {
  $json | Set-Content -Path $OutFile -Encoding UTF8
}
$json
