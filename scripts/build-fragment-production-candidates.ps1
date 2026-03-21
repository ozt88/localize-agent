param(
  [string]$InputPath = "c:\Users\DELL\Desktop\localize-agent\workflow\output\cluster_tier_report.json",
  [string]$OutputPath = "c:\Users\DELL\Desktop\localize-agent\workflow\output\fragment_production_candidates.json"
)

$ErrorActionPreference = "Stop"

function Get-RoleSet {
  param($lines)
  $set = New-Object 'System.Collections.Generic.HashSet[string]'
  foreach ($line in @($lines)) {
    if ($null -ne $line.role -and $line.role -ne '') {
      [void]$set.Add([string]$line.role)
    }
  }
  return @($set)
}

function Get-ShortLineCount {
  param($lines)
  $n = 0
  foreach ($line in @($lines)) {
    $en = [string]$line.en
    if (($en -split '\s+' | Where-Object { $_ -ne '' }).Count -le 3) {
      $n++
    }
  }
  return $n
}

function Test-BrokenSpeechFamily {
  param($cluster)
  $joined = [string]$cluster.joined_en
  $roles = Get-RoleSet $cluster.lines
  $shortCount = Get-ShortLineCount $cluster.lines
  if (($roles -contains 'dialogue' -or $roles -contains 'fragment') -and $shortCount -ge 2) {
    if ($joined.Contains('...') -or $joined.Contains('- ') -or $joined.Contains(' - ') -or $joined.Contains('?')) {
      return $true
    }
  }
  return $false
}

function Test-DramaticFragmentFamily {
  param($cluster)
  $joined = [string]$cluster.joined_en
  $roles = Get-RoleSet $cluster.lines
  $shortCount = Get-ShortLineCount $cluster.lines
  if (($roles -contains 'fragment' -or $roles -contains 'reaction' -or $roles -contains 'narration') -and $shortCount -ge 2) {
    if ($joined -match '(^|[ .])(SCARY|BROKEN|FEAR|ENVY|RESPECT|DIE|DEAD|WAIT|NOTHING|SOUND)([ .!?:]|$)') {
      return $true
    }
  }
  return $false
}

function Test-StaccatoEmphasisFamily {
  param($cluster)
  $joined = [string]$cluster.joined_en
  $shortCount = Get-ShortLineCount $cluster.lines
  if ($shortCount -ge 3) {
    if ($joined.Contains('[[E') -or $joined -match '\b[A-Z]{3,}\b' -or $joined.Contains('[T0]')) {
      return $true
    }
  }
  return $false
}

$raw = Get-Content $InputPath -Raw -Encoding utf8
$json = $raw | ConvertFrom-Json

$out = [ordered]@{
  input = $InputPath
  generated_at = (Get-Date).ToString("o")
  families = [ordered]@{
    broken_speech = @()
    dramatic_fragment = @()
    staccato_emphasis = @()
  }
}

foreach ($tierName in @('A','B','C')) {
  foreach ($cluster in @($json.clusters.$tierName)) {
    $entry = [ordered]@{
      tier = $tierName
      score = [int]$cluster.score
      ids = @($cluster.ids)
      joined_en = [string]$cluster.joined_en
      roles = @($cluster.roles)
    }
    if (Test-BrokenSpeechFamily $cluster) {
      $out.families.broken_speech += $entry
    }
    if (Test-DramaticFragmentFamily $cluster) {
      $out.families.dramatic_fragment += $entry
    }
    if (Test-StaccatoEmphasisFamily $cluster) {
      $out.families.staccato_emphasis += $entry
    }
  }
}

$out | ConvertTo-Json -Depth 8 | Set-Content $OutputPath -Encoding utf8
Write-Output $OutputPath
