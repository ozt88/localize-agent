Get-CimInstance Win32_Process |
  Where-Object {
    $_.Name -eq "go-translate.exe" -and
    $_.ExecutablePath -like "*projects\\esoteric-ebb\\.bin\\go-translate.exe"
  } |
  ForEach-Object {
    Stop-Process -Id $_.ProcessId -Force
  }
