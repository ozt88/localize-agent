param(
    [ValidateSet("sqlite", "postgres")]
    [string]$DBBackend = "postgres",
    [string]$DBDsn = "postgres://postgres@127.0.0.1:5433/localize_agent?sslmode=disable",
    [string]$OutDir = "projects/esoteric-ebb/patch/output/unified_patch_build",
    [string]$PluginProject = "projects/esoteric-ebb/patch/mod-loader/EsotericEbb.TranslationLoader/EsotericEbb.TranslationLoader.csproj",
    [string]$PluginDll = "projects/esoteric-ebb/patch/mod-loader/EsotericEbb.TranslationLoader/bin/Release/net6.0/EsotericEbb.TranslationLoader.dll",
    [string]$RuntimeBase = "projects/esoteric-ebb/patch/output/korean_patch_build_postgres/dist_full",
    [string]$TextAssetDir = "projects/esoteric-ebb/extract/1.1.1/ExportedProject/Assets/TextAsset",
    [string]$GameDir = "E:\SteamLibrary\steamapps\common\Esoteric Ebb",
    [switch]$SkipInstall,
    [switch]$IncludeDebugPlugins
)

# Unified patch build script — v2 pipeline (Go export)
#
# Uses go-v2-export to generate translations.json from pipeline_items_v2 table.
# This produces block-level source keys that match game rendering units.
#
# DO NOT use build_korean_patch_from_checkpoint.py — that reads the v1 'items'
# table with line-level IDs, producing translations the game cannot match.

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Resolve-BepInExReferenceRoot {
    param([string]$RuntimeBasePath)
    $candidate = Join-Path $RuntimeBasePath "BepInEx"
    if (Test-Path (Join-Path $candidate "core\BepInEx.Core.dll")) {
        return (Resolve-Path $candidate).Path
    }
    throw "BepInEx reference root not found under $RuntimeBasePath"
}

function Assert-Exists {
    param([string]$Path, [string]$Label)
    if (-not (Test-Path $Path)) {
        throw "$Label not found: $Path"
    }
}

function Get-FileSha256 {
    param([string]$Path)
    return (Get-FileHash -Algorithm SHA256 -Path $Path).Hash
}

function Write-InstallScripts {
    param([string]$DistPath)

    $installScript = @'
param([string]$GameDir = ".")

$ErrorActionPreference = "Stop"
$patchRoot = Split-Path -Parent $MyInvocation.MyCommand.Path

Copy-Item -Path (Join-Path $patchRoot "BepInEx") -Destination $GameDir -Recurse -Force
Copy-Item -Path (Join-Path $patchRoot "Esoteric Ebb_Data") -Destination $GameDir -Recurse -Force
Copy-Item -Path (Join-Path $patchRoot ".doorstop_version") -Destination $GameDir -Force
Copy-Item -Path (Join-Path $patchRoot "doorstop_config.ini") -Destination $GameDir -Force
Copy-Item -Path (Join-Path $patchRoot "winhttp.dll") -Destination $GameDir -Force
if (Test-Path (Join-Path $patchRoot "dotnet")) {
    Copy-Item -Path (Join-Path $patchRoot "dotnet") -Destination $GameDir -Recurse -Force
}

Write-Host "Patch installed to $GameDir"
'@
    Set-Content -Path (Join-Path $DistPath "install_patch.ps1") -Value $installScript -Encoding UTF8
}

# ── 1. Resolve paths ──

$runtimeBasePath = (Resolve-Path $RuntimeBase).Path
$bepInExReferenceRoot = Resolve-BepInExReferenceRoot -RuntimeBasePath $runtimeBasePath
$outDirPath = $OutDir

Write-Host "=== Unified Patch Build (v2 pipeline) ==="
Write-Host "DB backend: $DBBackend"
Write-Host "BepInEx reference: $bepInExReferenceRoot"

# ── 2. Build Plugin.dll ──

Write-Host "`n--- Building Plugin.dll ---"
dotnet build $PluginProject -c Release /p:BepInExRoot="$bepInExReferenceRoot"
if ($LASTEXITCODE -ne 0) { throw "Plugin build failed." }
Assert-Exists -Path $PluginDll -Label "Plugin DLL"
Write-Host "Plugin DLL: $PluginDll"

# ── 3. Export translations via Go v2 pipeline ──

Write-Host "`n--- Exporting translations (go-v2-export) ---"

$artifactsDir = Join-Path $outDirPath "artifacts"
New-Item -ItemType Directory -Force -Path $artifactsDir | Out-Null

$exportArgs = @(
    "run", "./workflow/cmd/go-v2-export",
    "-backend", $DBBackend,
    "-out-dir", $artifactsDir
)

if ($DBBackend -eq "postgres") {
    $exportArgs += @("-dsn", $DBDsn)
}

if (Test-Path $TextAssetDir) {
    $exportArgs += @("-textasset-dir", $TextAssetDir)
    Write-Host "TextAsset dir: $TextAssetDir"
}

& go @exportArgs
if ($LASTEXITCODE -ne 0) { throw "go-v2-export failed." }

Assert-Exists -Path (Join-Path $artifactsDir "translations.json") -Label "translations.json"
Write-Host "Export complete."

# ── 4. Assemble dist_full from runtime base + exported artifacts ──

Write-Host "`n--- Assembling dist_full ---"

$distPath = Join-Path $outDirPath "dist"
$fullPath = Join-Path $outDirPath "dist_full"

if (Test-Path $fullPath) {
    cmd /c rd /s /q "$fullPath" | Out-Null
}
New-Item -ItemType Directory -Force -Path $fullPath | Out-Null

# Start from runtime base (doorstop, dotnet, BepInEx core, fonts, etc.)
robocopy $runtimeBasePath $fullPath /MIR /NFL /NDL /NJH /NJS /NP | Out-Null

# Overlay exported artifacts (translations.json, textassets)
$exportedTranslations = Join-Path $artifactsDir "translations.json"
$patchDir = Join-Path $fullPath "Esoteric Ebb_Data\StreamingAssets\TranslationPatch"
New-Item -ItemType Directory -Force -Path $patchDir | Out-Null
Copy-Item $exportedTranslations (Join-Path $patchDir "translations.json") -Force

# Copy textasset overrides if exported
$exportedTextAssets = Join-Path $artifactsDir "textassets"
if (Test-Path $exportedTextAssets) {
    $textassetTarget = Join-Path $patchDir "textassets"
    New-Item -ItemType Directory -Force -Path $textassetTarget | Out-Null
    robocopy $exportedTextAssets $textassetTarget /E /NFL /NDL /NJH /NJS /NP | Out-Null
}

# Copy static localization overrides if present
$staticLocalization = "projects/esoteric-ebb/patch/output/static_reinject/localizationtexts_ko"
if (Test-Path $staticLocalization) {
    $locTarget = Join-Path $patchDir "localizationtexts"
    New-Item -ItemType Directory -Force -Path $locTarget | Out-Null
    robocopy (Resolve-Path $staticLocalization).Path $locTarget /E /NFL /NDL /NJH /NJS /NP | Out-Null
}

# Copy runtime_lexicon.json from patch input (canonical source)
$lexiconSource = "projects/esoteric-ebb/patch/input/runtime_lexicon.json"
if (Test-Path $lexiconSource) {
    Copy-Item $lexiconSource (Join-Path $patchDir "runtime_lexicon.json") -Force
}

# Overlay latest Plugin DLL
$fullPluginDir = Join-Path $fullPath "BepInEx\plugins\EsotericEbbTranslationLoader"
New-Item -ItemType Directory -Force -Path $fullPluginDir | Out-Null
Copy-Item $PluginDll (Join-Path $fullPluginDir "EsotericEbb.TranslationLoader.dll") -Force

# Debug plugins
if ($IncludeDebugPlugins) {
    $debugPlugins = "projects/esoteric-ebb/patch/debug/BepInEx/plugins"
    if (Test-Path $debugPlugins) {
        $debugTarget = Join-Path $fullPath "BepInEx\plugins"
        robocopy (Resolve-Path $debugPlugins).Path $debugTarget /E /NFL /NDL /NJH /NJS /NP | Out-Null
    }
}

# ── 5. Create slim dist (no runtime base, just patch files) ──

if (Test-Path $distPath) {
    cmd /c rd /s /q "$distPath" | Out-Null
}
New-Item -ItemType Directory -Force -Path $distPath | Out-Null

$distPluginDir = Join-Path $distPath "BepInEx\plugins\EsotericEbbTranslationLoader"
New-Item -ItemType Directory -Force -Path $distPluginDir | Out-Null
Copy-Item $PluginDll (Join-Path $distPluginDir "EsotericEbb.TranslationLoader.dll") -Force

$distPatchDir = Join-Path $distPath "Esoteric Ebb_Data\StreamingAssets\TranslationPatch"
New-Item -ItemType Directory -Force -Path $distPatchDir | Out-Null
Copy-Item $exportedTranslations (Join-Path $distPatchDir "translations.json") -Force

if (Test-Path $lexiconSource) {
    Copy-Item $lexiconSource (Join-Path $distPatchDir "runtime_lexicon.json") -Force
}

# ── 6. Verify required files ──

Write-Host "`n--- Verifying dist_full ---"

$required = @(
    @{ Path = (Join-Path $fullPath "BepInEx\plugins\EsotericEbbTranslationLoader\EsotericEbb.TranslationLoader.dll"); Label = "Plugin DLL" },
    @{ Path = (Join-Path $fullPath "Esoteric Ebb_Data\StreamingAssets\TranslationPatch\translations.json"); Label = "translations.json" },
    @{ Path = (Join-Path $fullPath "Esoteric Ebb_Data\StreamingAssets\TranslationPatch\runtime_lexicon.json"); Label = "runtime_lexicon.json" }
)

foreach ($req in $required) {
    Assert-Exists -Path $req.Path -Label $req.Label
    Write-Host "  OK: $($req.Label)"
}

# ── 7. Write manifest ──

$manifestPath = Join-Path $outDirPath "package_manifest.json"
$manifest = [ordered]@{
    generated_at = (Get-Date).ToString("o")
    pipeline = "v2 (go-v2-export)"
    db_backend = $DBBackend
    db = $(if ($DBBackend -eq "postgres") { $DBDsn } else { "sqlite" })
    plugin_dll = (Resolve-Path $PluginDll).Path
    plugin_dll_sha256 = (Get-FileSha256 -Path $PluginDll)
    translations_json = (Join-Path $fullPath "Esoteric Ebb_Data\StreamingAssets\TranslationPatch\translations.json")
    translations_json_sha256 = (Get-FileSha256 -Path (Join-Path $fullPath "Esoteric Ebb_Data\StreamingAssets\TranslationPatch\translations.json"))
    runtime_lexicon_present = (Test-Path (Join-Path $fullPath "Esoteric Ebb_Data\StreamingAssets\TranslationPatch\runtime_lexicon.json"))
    localizationtexts_present = (Test-Path (Join-Path $fullPath "Esoteric Ebb_Data\StreamingAssets\TranslationPatch\localizationtexts"))
    textasset_overrides_present = (Test-Path (Join-Path $fullPath "Esoteric Ebb_Data\StreamingAssets\TranslationPatch\textassets"))
    debug_plugins_included = [bool]$IncludeDebugPlugins
}

$manifest | ConvertTo-Json -Depth 5 | Set-Content -Path $manifestPath -Encoding UTF8
Write-Host "`nManifest: $manifestPath"

# ── 8. Write install scripts ──

Write-InstallScripts -DistPath $fullPath

# ── 9. Create zip ──

$zipPath = Join-Path $outDirPath "EsotericEbb_KoreanPatch.zip"
if (Test-Path $zipPath) { Remove-Item $zipPath -Force }
Add-Type -AssemblyName System.IO.Compression.FileSystem
[System.IO.Compression.ZipFile]::CreateFromDirectory($fullPath, $zipPath, [System.IO.Compression.CompressionLevel]::Optimal, $false)
Write-Host "Zip: $zipPath"

# ── 10. Install to game (unless --SkipInstall) ──

if (-not $SkipInstall -and (Test-Path $GameDir)) {
    Write-Host "`n--- Installing to game ---"

    # Plugin DLL
    $gamePluginDir = Join-Path $GameDir "BepInEx\plugins\EsotericEbbTranslationLoader"
    New-Item -ItemType Directory -Force -Path $gamePluginDir | Out-Null
    Copy-Item $PluginDll (Join-Path $gamePluginDir "EsotericEbb.TranslationLoader.dll") -Force
    Write-Host "  Installed: Plugin DLL"

    # TranslationPatch assets (translations.json, runtime_lexicon.json, textassets, etc.)
    $gamePatchDir = Join-Path $GameDir "Esoteric Ebb_Data\StreamingAssets\TranslationPatch"
    New-Item -ItemType Directory -Force -Path $gamePatchDir | Out-Null
    Copy-Item $exportedTranslations (Join-Path $gamePatchDir "translations.json") -Force
    Write-Host "  Installed: translations.json"

    if (Test-Path $lexiconSource) {
        Copy-Item $lexiconSource (Join-Path $gamePatchDir "runtime_lexicon.json") -Force
        Write-Host "  Installed: runtime_lexicon.json"
    }

    if (Test-Path $exportedTextAssets) {
        $gameTextAssetDir = Join-Path $gamePatchDir "textassets"
        New-Item -ItemType Directory -Force -Path $gameTextAssetDir | Out-Null
        robocopy $exportedTextAssets $gameTextAssetDir /E /NFL /NDL /NJH /NJS /NP | Out-Null
        Write-Host "  Installed: textasset overrides"
    }

    # Clear stale state to force fresh metrics
    $staleFiles = @(
        (Join-Path $GameDir "BepInEx\translation_loader_state.json"),
        (Join-Path $GameDir "BepInEx\untranslated_capture.json")
    )
    foreach ($f in $staleFiles) {
        if (Test-Path $f) {
            Remove-Item $f -Force
            Write-Host "  Cleared: $(Split-Path -Leaf $f)"
        }
    }

    Write-Host "  Install complete: $GameDir"
} elseif ($SkipInstall) {
    Write-Host "`nSkipped game install (--SkipInstall)."
} else {
    Write-Host "`nGame dir not found, skipped install: $GameDir"
}

Write-Host "`n=== Build complete ==="
Write-Host "dist_full: $fullPath"
