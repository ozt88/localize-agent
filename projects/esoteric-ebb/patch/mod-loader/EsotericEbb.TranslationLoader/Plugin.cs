using System.Reflection;
using System.Runtime.CompilerServices;
using System.Runtime.InteropServices;
using System.Text.Json;
using System.Text.RegularExpressions;
using BepInEx;
using BepInEx.Logging;
using BepInEx.Unity.IL2CPP;
using HarmonyLib;

namespace EsotericEbb.TranslationLoader;

/// <summary>
/// Plugin.cs v2 — Complete rewrite for v2 translation pipeline.
/// Zero v1 legacy code. V3 sidecar format only. 4-stage TryTranslate chain.
/// </summary>
[BepInPlugin(PluginGuid, PluginName, PluginVersion)]
public class Plugin : BasePlugin
{
    // =========================================================================
    // Constants
    // =========================================================================

    public const string PluginGuid = "com.esotericebb.translationloader";
    public const string PluginName = "EsotericEbb Translation Loader";
    public const string PluginVersion = "2.0.0";

    private const string V3SidecarFormat = "esoteric-ebb-sidecar.v3";
    private const string LexiconV2Format = "esoteric-ebb-runtime-lexicon.v2";
    private const string EnableFullCaptureFlag = "ENABLE_FULL_CAPTURE";
    private const int RecentHistoryLimit = 8;

    // =========================================================================
    // Fields — Data Stores
    // =========================================================================

    internal static ManualLogSource? LogSource;

    /// <summary>Exact source->target map (deduped by source, first-seen-wins). Per D-03: exact match only.</summary>
    internal static readonly Dictionary<string, string> TranslationMap = new(StringComparer.Ordinal);

    /// <summary>Source text -> list of contextual candidates with metadata. Per D-04.</summary>
    private static readonly Dictionary<string, List<ContextualEntry>> ContextualMap = new(StringComparer.Ordinal);

    /// <summary>TextAsset name -> replacement content. OrdinalIgnoreCase for Unity asset name matching.</summary>
    private static readonly Dictionary<string, string> TextAssetOverrides = new(StringComparer.OrdinalIgnoreCase);

    /// <summary>I2 localization ID -> Korean translation.</summary>
    private static readonly Dictionary<string, string> LocalizationIdOverrides = new(StringComparer.OrdinalIgnoreCase);

    /// <summary>Runtime lexicon v2: exact find->replace.</summary>
    private static readonly Dictionary<string, string> RuntimeExactReplacements = new(StringComparer.Ordinal);

    /// <summary>Runtime lexicon: substring find->replace, sorted by find length descending.</summary>
    private static readonly List<KeyValuePair<string, string>> RuntimeSubstringReplacements = new();

    /// <summary>Runtime lexicon: compiled regex rules.</summary>
    private static readonly List<RuntimeRegexRule> RuntimeRegexRules = new();

    // =========================================================================
    // Fields — Counters & State
    // =========================================================================

    private static int _translationMapHits;
    private static int _dcfcStripHits;
    private static int _contextualHits;
    private static int _runtimeLexiconHits;
    private static int _misses;
    private static int _totalTranslateAttempts;
    private static int _lastFlushAttempt;
    private static int _textAssetOverrideHits;
    private static int _localizationIdOverrideHits;
    private static int _contextualLoadedCount;

    // =========================================================================
    // Fields — Context Tracking
    // =========================================================================

    private static string _currentDialogSourceFile = "";
    private static readonly HashSet<string> _recentContextHistory = new(StringComparer.Ordinal);

    // =========================================================================
    // Fields — Capture Infrastructure (D-12)
    // =========================================================================

    private static readonly HashSet<string> _untranslatedCapture = new(StringComparer.Ordinal);
    private static readonly Dictionary<string, string> _translationHitsCapture = new(StringComparer.Ordinal);
    private static bool _fullCaptureEnabled;
    private static string? _capturePath;
    private static string? _translationHitLogPath;
    private static string? _statePath;

    // =========================================================================
    // Fields — Patch State
    // =========================================================================

    [ThreadStatic] private static int _patchReentryDepth;

    private Harmony? _harmony;
    private bool _tmpTextPatched;
    private bool _textAssetPatched;
    private bool _dialogPatched;
    private bool _localizePatched;
    private bool _tmpOnEnablePatched;
    private bool _tmpUguiAwakePatched;
    private bool _tmpWorldAwakePatched;
    private bool _dialogStartPatched;
    private bool _uiTextPatched;
    private bool _uiOnEnablePatched;
    private bool _menuStartPatched;
    private bool _menuRefreshPatched;
    private bool _sceneLoadedPatched;

    // =========================================================================
    // Fields — Font
    // =========================================================================

    private static object? _koreanFontAsset;
    private static bool _fontLoaded;
    private static readonly HashSet<int> _fontInjectedAssets = new();
    private static string _fontStatus = "not_attempted";

    [DllImport("gdi32.dll", CharSet = CharSet.Unicode)]
    private static extern int AddFontResourceEx(string lpFileName, uint fl, IntPtr pdv);

    // =========================================================================
    // Fields — Misc
    // =========================================================================

    private static readonly HashSet<string> TextAssetOverrideMissSeen = new(StringComparer.OrdinalIgnoreCase);
    private static int _textAssetOverrideMissLoggedCount;
    private static readonly HashSet<string> _triggeredSceneScans = new(StringComparer.Ordinal);
    private static readonly object StateLock = new();
    private static readonly Dictionary<string, Type?> TypeCache = new(StringComparer.Ordinal);

    // DC/FC prefix regex — must match export.go dcfcPrefixRe exactly
    private static readonly Regex DcFcPrefixRegex = new(@"^[A-Z]{2}\d+\s+\w+-", RegexOptions.Compiled);

    // Choice text arrives pre-wrapped: <#FFFFFFFF><line-indent=-10%><link="N">N.   BODY</link></line-indent></color>
    // We need to extract BODY, translate it, and re-wrap.
    private static readonly Regex ChoiceWrapperRegex = new(
        @"^((?:<[^>]+>)*\d+\.\s+)(.*?)(</link>.*)$",
        RegexOptions.Compiled | RegexOptions.Singleline);

    // =========================================================================
    // Load() Entry Point
    // =========================================================================

    public override void Load()
    {
        LogSource = Log;

        // Resolve TranslationPatch directory
        var basePath = Path.Combine(Paths.GameRootPath, "Esoteric Ebb_Data", "StreamingAssets", "TranslationPatch");
        if (!Directory.Exists(basePath))
        {
            basePath = Path.Combine(Paths.GameRootPath, "StreamingAssets", "TranslationPatch");
        }

        // Setup capture paths
        _capturePath = Path.Combine(Paths.GameRootPath, "BepInEx", "untranslated_capture.json");
        _translationHitLogPath = Path.Combine(Paths.GameRootPath, "BepInEx", "translation_hits.json");
        _statePath = Path.Combine(Paths.GameRootPath, "BepInEx", "translation_loader_state.json");
        _fullCaptureEnabled = File.Exists(Path.Combine(Paths.GameRootPath, "BepInEx", EnableFullCaptureFlag));

        // Load data
        LoadTranslations(basePath);
        LoadTextAssetOverrides(basePath);
        LoadLocalizationIdOverrides(basePath);
        LoadRuntimeLexicon(basePath);

        // Setup Harmony
        _harmony = new Harmony(PluginGuid);
        TryApplyPatches();

        // Deferred patching for assemblies not yet loaded
        AppDomain.CurrentDomain.AssemblyLoad += OnAssemblyLoad;

        var contextualTotal = 0;
        foreach (var list in ContextualMap.Values)
            contextualTotal += list.Count;

        var lexiconRules = RuntimeExactReplacements.Count + RuntimeSubstringReplacements.Count + RuntimeRegexRules.Count;

        Log.LogInfo($"v2 loaded: {TranslationMap.Count} entries, {contextualTotal} contextual, " +
                    $"{TextAssetOverrides.Count} textassets, {LocalizationIdOverrides.Count} localization IDs, " +
                    $"{lexiconRules} lexicon rules");

        if (_fullCaptureEnabled)
        {
            Log.LogInfo("FULL CAPTURE MODE enabled — translation hits will be logged");
        }

        WriteState("load_complete");

        // Register quit handler for final state flush
        try
        {
            var appType = FindTypeByName("UnityEngine.Application");
            var quittingEvent = appType?.GetEvent("quitting", BindingFlags.Public | BindingFlags.Static);
            if (quittingEvent != null)
            {
                var handler = new Action(OnApplicationQuitting);
                quittingEvent.AddMethod?.Invoke(null, new object[] { handler });
                Log.LogInfo("Registered Application.quitting handler for capture flush");
            }
        }
        catch (Exception ex)
        {
            Log.LogWarning($"Could not register quit handler: {ex.Message}");
        }
    }

    private static void OnApplicationQuitting()
    {
        WriteUntranslatedCapture();
        WriteTranslationHits();
        WriteState("quit");
        LogSource?.LogInfo($"Final state flushed on quit: {_translationMapHits} exact, {_dcfcStripHits} dcfc, {_contextualHits} contextual, {_runtimeLexiconHits} lexicon, {_misses} misses");
    }

    // =========================================================================
    // Data Loading
    // =========================================================================

    private void LoadTranslations(string basePath)
    {
        var candidates = new[]
        {
            Path.Combine(basePath, "translations.json"),
            Path.Combine(Paths.GameRootPath, "StreamingAssets", "TranslationPatch", "translations.json")
        };

        var path = candidates.FirstOrDefault(File.Exists);
        if (path is null)
        {
            Log.LogWarning("translations.json not found");
            return;
        }

        try
        {
            var json = File.ReadAllText(path);
            LoadEntriesFromJson(json);
            Log.LogInfo($"Loaded translations: {path} ({TranslationMap.Count} entries, {_contextualLoadedCount} contextual)");
        }
        catch (Exception ex)
        {
            Log.LogError($"Failed to load translations: {ex.Message}");
        }
    }

    /// <summary>
    /// V3 sidecar format ONLY. No v1 flat array fallback. Per D-03: exact match only.
    /// </summary>
    private void LoadEntriesFromJson(string json)
    {
        using var doc = JsonDocument.Parse(json, new JsonDocumentOptions
        {
            CommentHandling = JsonCommentHandling.Skip,
            AllowTrailingCommas = true
        });

        var root = doc.RootElement;
        if (root.ValueKind != JsonValueKind.Object)
        {
            Log.LogError("translations.json: expected JSON object (v3 sidecar), got " + root.ValueKind);
            return;
        }

        // Validate format
        var format = ReadString(root, "format") ?? "";
        if (format != V3SidecarFormat)
        {
            Log.LogError($"translations.json: expected format '{V3SidecarFormat}', got '{format}'");
            return;
        }

        // entries[] -> TranslationMap (exact match, source -> target)
        if (root.TryGetProperty("entries", out var entries) && entries.ValueKind == JsonValueKind.Array)
        {
            foreach (var item in entries.EnumerateArray())
            {
                var source = ReadString(item, "source");
                var target = ReadString(item, "target");
                if (!string.IsNullOrEmpty(source) && !string.IsNullOrEmpty(target))
                {
                    TranslationMap[source] = target;
                }
            }
        }

        // contextual_entries[] -> ContextualMap (grouped by source text)
        if (root.TryGetProperty("contextual_entries", out var contextualEntries) &&
            contextualEntries.ValueKind == JsonValueKind.Array)
        {
            foreach (var item in contextualEntries.EnumerateArray())
            {
                AddContextualEntry(item);
            }
        }
    }

    private static void AddContextualEntry(JsonElement item)
    {
        var source = ReadString(item, "source");
        var target = ReadString(item, "target");
        if (string.IsNullOrWhiteSpace(source) || string.IsNullOrWhiteSpace(target))
        {
            return;
        }

        if (!ContextualMap.TryGetValue(source, out var entryList))
        {
            entryList = new List<ContextualEntry>();
            ContextualMap[source] = entryList;
        }

        entryList.Add(new ContextualEntry
        {
            Source = source,
            Target = target,
            SourceFile = ReadString(item, "source_file") ?? "",
            TextRole = ReadString(item, "text_role") ?? "",
            SpeakerHint = ReadString(item, "speaker_hint") ?? ""
        });
        _contextualLoadedCount++;
    }

    private void LoadTextAssetOverrides(string basePath)
    {
        var candidateDirs = new[]
        {
            Path.Combine(basePath, "textassets"),
            Path.Combine(basePath, "localizationtexts"),
            Path.Combine(Paths.GameRootPath, "StreamingAssets", "TranslationPatch", "textassets"),
            Path.Combine(Paths.GameRootPath, "StreamingAssets", "TranslationPatch", "localizationtexts")
        };

        var dirs = candidateDirs.Where(Directory.Exists).Distinct(StringComparer.OrdinalIgnoreCase).ToArray();
        if (dirs.Length == 0)
        {
            return;
        }

        try
        {
            foreach (var dir in dirs)
            {
                foreach (var pattern in new[] { "*.txt", "*.json" })
                {
                    foreach (var filePath in Directory.EnumerateFiles(dir, pattern, SearchOption.AllDirectories))
                    {
                        var name = Path.GetFileNameWithoutExtension(filePath);
                        var text = File.ReadAllText(filePath);
                        if (!string.IsNullOrWhiteSpace(name) && !string.IsNullOrWhiteSpace(text))
                        {
                            TextAssetOverrides[name] = text;
                        }
                    }
                }
            }

            Log.LogInfo($"Loaded text asset overrides from {dirs.Length} directories ({TextAssetOverrides.Count} files)");
        }
        catch (Exception ex)
        {
            Log.LogWarning($"Failed to load text asset overrides: {ex.Message}");
        }
    }

    private void LoadLocalizationIdOverrides(string basePath)
    {
        var candidates = new[]
        {
            Path.Combine(basePath, "localizationtexts"),
            Path.Combine(Paths.GameRootPath, "StreamingAssets", "TranslationPatch", "localizationtexts")
        };

        var dir = candidates.FirstOrDefault(Directory.Exists);
        if (dir is null)
        {
            return;
        }

        try
        {
            foreach (var path in Directory.EnumerateFiles(dir, "*.txt"))
            {
                foreach (var line in File.ReadLines(path))
                {
                    if (string.IsNullOrWhiteSpace(line) || line.StartsWith("ID,", StringComparison.OrdinalIgnoreCase))
                    {
                        continue;
                    }

                    var parts = ParseCsvLine(line);
                    if (parts.Count < 3)
                    {
                        continue;
                    }

                    var id = parts[0].Trim();
                    var ko = parts[2].Trim();
                    if (!string.IsNullOrWhiteSpace(id) && !string.IsNullOrWhiteSpace(ko))
                    {
                        LocalizationIdOverrides[id] = ko;
                    }
                }
            }

            Log.LogInfo($"Loaded localization ID overrides: ({LocalizationIdOverrides.Count} IDs)");
        }
        catch (Exception ex)
        {
            Log.LogWarning($"Failed to load localization ID overrides: {ex.Message}");
        }
    }

    private void LoadRuntimeLexicon(string basePath)
    {
        var candidates = new[]
        {
            Path.Combine(basePath, "runtime_lexicon.json"),
            Path.Combine(Paths.GameRootPath, "StreamingAssets", "TranslationPatch", "runtime_lexicon.json")
        };

        var path = candidates.FirstOrDefault(File.Exists);
        if (path is null)
        {
            return;
        }

        try
        {
            RuntimeExactReplacements.Clear();
            RuntimeSubstringReplacements.Clear();
            RuntimeRegexRules.Clear();

            using var doc = JsonDocument.Parse(File.ReadAllText(path));
            if (doc.RootElement.ValueKind != JsonValueKind.Object)
            {
                return;
            }

            // Validate format
            var format = ReadString(doc.RootElement, "format") ?? "";
            if (format != LexiconV2Format)
            {
                Log.LogWarning($"runtime_lexicon.json: expected '{LexiconV2Format}', got '{format}'. exact_replacements may be missing.");
            }

            // exact_replacements[] (v2 addition)
            if (doc.RootElement.TryGetProperty("exact_replacements", out var exactReplacements) &&
                exactReplacements.ValueKind == JsonValueKind.Array)
            {
                foreach (var item in exactReplacements.EnumerateArray())
                {
                    var find = ReadString(item, "find");
                    var replace = ReadString(item, "replace");
                    if (!string.IsNullOrEmpty(find) && replace is not null)
                    {
                        RuntimeExactReplacements[find] = replace;
                    }
                }
            }

            // substring_replacements[]
            if (doc.RootElement.TryGetProperty("substring_replacements", out var substringReplacements) &&
                substringReplacements.ValueKind == JsonValueKind.Array)
            {
                foreach (var item in substringReplacements.EnumerateArray())
                {
                    var find = ReadString(item, "find");
                    var replace = ReadString(item, "replace");
                    if (!string.IsNullOrEmpty(find) && replace is not null)
                    {
                        RuntimeSubstringReplacements.Add(new KeyValuePair<string, string>(find, replace));
                    }
                }
            }

            // Sort by find length descending for longest-match-first
            RuntimeSubstringReplacements.Sort((a, b) => b.Key.Length.CompareTo(a.Key.Length));

            // regex_rules[]
            if (doc.RootElement.TryGetProperty("regex_rules", out var regexRules) &&
                regexRules.ValueKind == JsonValueKind.Array)
            {
                foreach (var item in regexRules.EnumerateArray())
                {
                    var pattern = ReadString(item, "pattern");
                    var replace = ReadString(item, "replace");
                    if (string.IsNullOrEmpty(pattern) || replace is null)
                    {
                        continue;
                    }

                    var ignoreCase = item.TryGetProperty("ignore_case", out var ignoreCaseEl) &&
                                     ignoreCaseEl.ValueKind == JsonValueKind.True;
                    var options = RegexOptions.CultureInvariant | RegexOptions.Compiled;
                    if (ignoreCase)
                    {
                        options |= RegexOptions.IgnoreCase;
                    }

                    RuntimeRegexRules.Add(new RuntimeRegexRule
                    {
                        Name = ReadString(item, "name") ?? "",
                        Pattern = new Regex(pattern, options),
                        ReplaceString = replace
                    });
                }
            }

            Log.LogInfo($"Loaded runtime lexicon: {path} ({RuntimeExactReplacements.Count} exact, " +
                        $"{RuntimeSubstringReplacements.Count} substrings, {RuntimeRegexRules.Count} regex rules)");
        }
        catch (Exception ex)
        {
            Log.LogWarning($"Failed to load runtime lexicon: {ex.Message}");
        }
    }

    // =========================================================================
    // Harmony Patch Registration
    // =========================================================================

    private void OnAssemblyLoad(object? sender, AssemblyLoadEventArgs args)
    {
        TryApplyPatches();
    }

    private void TryApplyPatches()
    {
        if (_harmony is null) return;

        // 1. TMP_Text.text setter prefix
        if (!_tmpTextPatched)
        {
            _tmpTextPatched = ApplyTextPatch(
                new[] { "TMPro", "Unity.TextMeshPro" },
                "TMPro.TMP_Text", "text", nameof(TmpTextPrefix));
        }

        // 2. TextAsset.text getter postfix
        if (!_textAssetPatched)
        {
            _textAssetPatched = ApplyTextAssetPatch();
        }

        // 3. DialogManager.AddChoiceText prefix (D-14: text only)
        if (!_dialogPatched)
        {
            _dialogPatched = ApplyMethodPatch(
                new[] { "Assembly-CSharp" },
                "DialogManager", "AddChoiceText",
                nameof(DialogAddChoiceTextPrefix), parameterCount: 9, usePrefix: true);
        }

        // 4. Localize.CheckLanguage postfix (I2 localization)
        if (!_localizePatched)
        {
            _localizePatched = ApplyMethodPatch(
                new[] { "Assembly-CSharp", "I2Localization" },
                "I2.Loc.LocalizationManager", "GetTranslation",
                nameof(LocalizeCheckLanguagePostfix), parameterCount: 4, usePrefix: false);
        }

        // 5. TMP_Text.OnEnable postfix
        if (!_tmpOnEnablePatched)
        {
            _tmpOnEnablePatched = ApplyMethodPatch(
                new[] { "TMPro", "Unity.TextMeshPro" },
                "TMPro.TMP_Text", "OnEnable",
                nameof(TmpOnEnablePostfix), parameterCount: 0, usePrefix: false);
        }

        // 6a. TextMeshProUGUI.Awake postfix
        if (!_tmpUguiAwakePatched)
        {
            _tmpUguiAwakePatched = ApplyMethodPatch(
                new[] { "TMPro", "Unity.TextMeshPro" },
                "TMPro.TextMeshProUGUI", "Awake",
                nameof(TmpConcreteAwakePostfix), parameterCount: 0, usePrefix: false);
        }

        // 6b. TextMeshPro.Awake postfix
        if (!_tmpWorldAwakePatched)
        {
            _tmpWorldAwakePatched = ApplyMethodPatch(
                new[] { "TMPro", "Unity.TextMeshPro" },
                "TMPro.TextMeshPro", "Awake",
                nameof(TmpConcreteAwakePostfix), parameterCount: 0, usePrefix: false);
        }

        // 7. DialogManager.StartDialog prefix (sets _currentDialogSourceFile for ContextualMap)
        if (!_dialogStartPatched)
        {
            _dialogStartPatched = ApplyDialogStartPatch();
        }

        // 8. UI.Text.text setter prefix (legacy UI)
        if (!_uiTextPatched)
        {
            _uiTextPatched = ApplyTextPatch(
                new[] { "UnityEngine.UI", "UnityEngine.UIModule" },
                "UnityEngine.UI.Text", "text", nameof(UiTextPrefix));
        }

        // 9. UI.Text.OnEnable postfix
        if (!_uiOnEnablePatched)
        {
            _uiOnEnablePatched = ApplyMethodPatch(
                new[] { "UnityEngine.UI", "UnityEngine.UIModule" },
                "UnityEngine.UI.Text", "OnEnable",
                nameof(UiOnEnablePostfix), parameterCount: 0, usePrefix: false);
        }

        // 10. MenuController.Start postfix
        if (!_menuStartPatched)
        {
            _menuStartPatched = ApplyMethodPatch(
                new[] { "Assembly-CSharp" },
                "MenuController", "Start",
                nameof(MenuControllerStartPostfix), parameterCount: 0, usePrefix: false);
        }

        // 11. MenuController.RefreshMenuState postfix
        if (!_menuRefreshPatched)
        {
            _menuRefreshPatched = ApplyMethodPatch(
                new[] { "Assembly-CSharp" },
                "MenuController", "RefreshMenuState",
                nameof(MenuControllerRefreshPostfix), parameterCount: 1, usePrefix: false);
        }

        // 12. SceneManager.sceneLoaded postfix
        if (!_sceneLoadedPatched)
        {
            _sceneLoadedPatched = ApplySceneLoadedPatch();
        }
    }

    // =========================================================================
    // Patch Helpers
    // =========================================================================

    private bool ApplyTextPatch(string[] assemblyCandidates, string typeName, string propertyName, string patchMethodName)
    {
        try
        {
            var type = FindTypeInAssemblies(assemblyCandidates, typeName);
            if (type is null) return false;

            var setter = type.GetProperty(propertyName, BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic)?.SetMethod;
            if (setter is null)
            {
                Log.LogWarning($"Patch skipped: setter not found ({typeName}.{propertyName})");
                return false;
            }

            var prefix = typeof(Plugin).GetMethod(patchMethodName, BindingFlags.Static | BindingFlags.NonPublic);
            if (prefix is null) return false;

            _harmony!.Patch(setter, prefix: new HarmonyMethod(prefix));
            Log.LogInfo($"Patch applied: {typeName}.{propertyName} setter");
            return true;
        }
        catch (Exception ex)
        {
            Log.LogError($"Patch failed: {typeName}.{propertyName} => {ex.GetType().Name}: {ex.Message}");
            return false;
        }
    }

    private bool ApplyTextAssetPatch()
    {
        try
        {
            var type = FindTypeInAssemblies(
                new[] { "UnityEngine.TextRenderingModule", "UnityEngine.CoreModule", "UnityEngine" },
                "UnityEngine.TextAsset");
            if (type is null) return false;

            var getter = type.GetProperty("text", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic)?.GetMethod;
            if (getter is null)
            {
                Log.LogWarning("Patch skipped: getter not found (UnityEngine.TextAsset.text)");
                return false;
            }

            var postfix = typeof(Plugin).GetMethod(nameof(TextAssetTextPostfix), BindingFlags.Static | BindingFlags.NonPublic);
            if (postfix is null) return false;

            _harmony!.Patch(getter, postfix: new HarmonyMethod(postfix));
            Log.LogInfo("Patch applied: UnityEngine.TextAsset.text getter");
            return true;
        }
        catch (Exception ex)
        {
            Log.LogError($"Patch failed: TextAsset.text => {ex.GetType().Name}: {ex.Message}");
            return false;
        }
    }

    private bool ApplyMethodPatch(string[] assemblyCandidates, string typeName, string methodName,
        string patchMethodName, int parameterCount, bool usePrefix)
    {
        try
        {
            var type = FindTypeInAssemblies(assemblyCandidates, typeName);
            if (type is null) return false;

            var method = type.GetMethods(BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic)
                .FirstOrDefault(m => string.Equals(m.Name, methodName, StringComparison.Ordinal) &&
                                     m.GetParameters().Length == parameterCount);
            if (method is null)
            {
                Log.LogWarning($"Patch skipped: method not found ({typeName}.{methodName}/{parameterCount})");
                return false;
            }

            var patchMethod = typeof(Plugin).GetMethod(patchMethodName, BindingFlags.Static | BindingFlags.NonPublic);
            if (patchMethod is null) return false;

            if (usePrefix)
            {
                _harmony!.Patch(method, prefix: new HarmonyMethod(patchMethod));
            }
            else
            {
                _harmony!.Patch(method, postfix: new HarmonyMethod(patchMethod));
            }

            Log.LogInfo($"Patch applied: {typeName}.{methodName}/{parameterCount}");
            return true;
        }
        catch (Exception ex)
        {
            Log.LogError($"Patch failed: {typeName}.{methodName}/{parameterCount} => {ex.GetType().Name}: {ex.Message}");
            return false;
        }
    }

    private bool ApplyDialogStartPatch()
    {
        try
        {
            var type = FindTypeInAssemblies(new[] { "Assembly-CSharp" }, "DialogManager");
            if (type is null) return false;

            // StartDialog typically has an ink asset parameter
            var method = type.GetMethods(BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic)
                .FirstOrDefault(m => string.Equals(m.Name, "StartDialog", StringComparison.Ordinal));
            if (method is null) return false;

            var prefix = typeof(Plugin).GetMethod(nameof(DialogStartDialogPrefix), BindingFlags.Static | BindingFlags.NonPublic);
            if (prefix is null) return false;

            _harmony!.Patch(method, prefix: new HarmonyMethod(prefix));
            Log.LogInfo($"Patch applied: DialogManager.StartDialog");
            return true;
        }
        catch (Exception ex)
        {
            Log.LogError($"Patch failed: DialogManager.StartDialog => {ex.GetType().Name}: {ex.Message}");
            return false;
        }
    }

    private bool ApplySceneLoadedPatch()
    {
        try
        {
            var sceneManagerType = FindTypeByName("UnityEngine.SceneManagement.SceneManager");
            if (sceneManagerType is null) return false;

            var sceneLoadedEvent = sceneManagerType.GetEvent("sceneLoaded", BindingFlags.Public | BindingFlags.Static);
            if (sceneLoadedEvent is null) return false;

            var handler = typeof(Plugin).GetMethod(nameof(SceneLoadedPostfix), BindingFlags.Static | BindingFlags.NonPublic);
            if (handler is null) return false;

            var delegateType = sceneLoadedEvent.EventHandlerType;
            if (delegateType is null) return false;

            var del = Delegate.CreateDelegate(delegateType, handler);
            sceneLoadedEvent.AddMethod?.Invoke(null, new object[] { del });

            Log.LogInfo("Patch applied: SceneManager.sceneLoaded event");
            return true;
        }
        catch (Exception ex)
        {
            Log.LogError($"Patch failed: SceneManager.sceneLoaded => {ex.GetType().Name}: {ex.Message}");
            return false;
        }
    }

    // =========================================================================
    // Font Loading (D-11: Regular font priority)
    // =========================================================================

    private static void TryLoadKoreanFont()
    {
        if (_fontLoaded) return;
        _fontLoaded = true;

        try
        {
            var fontPath = FindKoreanFontFile();
            if (fontPath is null)
            {
                _fontStatus = "font_file_not_found";
                LogSource?.LogWarning("[Font] Korean font file not found");
                return;
            }

            LogSource?.LogInfo($"[Font] Found font file: {fontPath}");

            var fontType = FindTypeByName("UnityEngine.Font");
            if (fontType is null)
            {
                _fontStatus = "unity_font_type_not_found";
                return;
            }

            var tmpFontType = FindTypeByName("TMPro.TMP_FontAsset");
            if (tmpFontType is null)
            {
                _fontStatus = "tmp_font_type_not_found";
                return;
            }

            // Create Unity Font
            object? unityFont = TryCreateFontViaConstructor(fontType, fontPath);

            if (unityFont is null)
            {
                var added = AddFontResourceEx(fontPath, 0, IntPtr.Zero);
                if (added > 0)
                {
                    LogSource?.LogInfo($"[Font] AddFontResourceEx registered {added} font(s)");
                    unityFont = TryCreateFontFromOS(fontType, fontPath);
                }
            }

            if (unityFont is null)
            {
                unityFont = TryCreateFontDefault(fontType);
            }

            if (unityFont is null)
            {
                _fontStatus = "all_font_creation_strategies_failed";
                LogSource?.LogError("[Font] All font creation strategies failed");
                return;
            }

            SetObjectHideFlags(unityFont, 52); // DontUnloadUnusedAsset

            _koreanFontAsset = TryCreateTmpFontAsset(tmpFontType, fontType, unityFont);
            if (_koreanFontAsset is null)
            {
                _fontStatus = "create_font_asset_failed";
                LogSource?.LogWarning("[Font] Failed to create TMP_FontAsset");
                return;
            }

            SetObjectHideFlags(_koreanFontAsset, 52);
            _fontStatus = "loaded";
            LogSource?.LogInfo("[Font] Korean TMP_FontAsset created successfully");
        }
        catch (Exception ex)
        {
            var inner = ex.InnerException ?? ex;
            _fontStatus = $"error:{inner.GetType().Name}:{inner.Message}";
            LogSource?.LogError($"[Font] Failed to load Korean font: {inner}");
        }
    }

    private static string? FindKoreanFontFile()
    {
        var searchDirs = new[]
        {
            Path.Combine(Paths.GameRootPath, "Esoteric Ebb_Data", "StreamingAssets", "TranslationPatch", "fonts"),
            Path.Combine(Paths.GameRootPath, "StreamingAssets", "TranslationPatch", "fonts"),
            Path.Combine(Paths.GameRootPath, "BepInEx", "plugins", "fonts")
        };

        var filePatterns = new[] { "Pretendard*", "YeolrinMyeongjo*", "NanumMyeongjo*", "NEXON_Warhaven*" };

        foreach (var dir in searchDirs)
        {
            if (!Directory.Exists(dir)) continue;

            foreach (var pattern in filePatterns)
            {
                var files = Directory.GetFiles(dir, pattern)
                    .Where(f => f.EndsWith(".ttf", StringComparison.OrdinalIgnoreCase) ||
                                f.EndsWith(".otf", StringComparison.OrdinalIgnoreCase))
                    .ToArray();

                if (files.Length > 0)
                {
                    // Prefer Regular weight (D-11, Phase 4 fix e90555c)
                    var regular = files.FirstOrDefault(f =>
                        f.Contains("Regular", StringComparison.OrdinalIgnoreCase));
                    return regular ?? files[0];
                }
            }
        }

        return null;
    }

    private static object? TryCreateFontViaConstructor(Type fontType, string fontPath)
    {
        try
        {
            var ctor = fontType.GetConstructor(new[] { typeof(string) });
            if (ctor is not null)
            {
                var font = ctor.Invoke(new object[] { fontPath });
                if (font is not null)
                {
                    LogSource?.LogInfo("[Font] Font(string path) constructor succeeded");
                    return font;
                }
            }
        }
        catch (Exception ex)
        {
            LogSource?.LogInfo($"[Font] Font(string) constructor failed: {ex.InnerException?.Message ?? ex.Message}");
        }

        return null;
    }

    private static object? TryCreateFontFromOS(Type fontType, string fontPath)
    {
        // Discover font name from OS
        string? discoveredName = null;
        try
        {
            var getNamesMethod = fontType.GetMethod("GetOSInstalledFontNames", BindingFlags.Public | BindingFlags.Static);
            if (getNamesMethod?.Invoke(null, null) is string[] fontNames)
            {
                discoveredName = fontNames.FirstOrDefault(n => n.Contains("Pretendard", StringComparison.OrdinalIgnoreCase))
                              ?? fontNames.FirstOrDefault(n => n.Contains("Warhaven", StringComparison.OrdinalIgnoreCase));
            }
        }
        catch { }

        var candidateNames = new List<string>();
        if (discoveredName is not null) candidateNames.Add(discoveredName);
        var baseName = Path.GetFileNameWithoutExtension(fontPath);
        candidateNames.Add(baseName);

        var createMethod = fontType.GetMethod("CreateDynamicFontFromOSFont",
            BindingFlags.Public | BindingFlags.Static, null, new[] { typeof(string), typeof(int) }, null);
        if (createMethod is null) return null;

        foreach (var candidate in candidateNames)
        {
            try
            {
                var font = createMethod.Invoke(null, new object[] { candidate, 24 });
                if (font is not null)
                {
                    LogSource?.LogInfo($"[Font] CreateDynamicFontFromOSFont succeeded with: {candidate}");
                    return font;
                }
            }
            catch (Exception ex)
            {
                LogSource?.LogInfo($"[Font] CreateDynamicFontFromOSFont failed for '{candidate}': {ex.InnerException?.Message ?? ex.Message}");
            }
        }

        return null;
    }

    private static object? TryCreateFontDefault(Type fontType)
    {
        try
        {
            var ctor = fontType.GetConstructor(Type.EmptyTypes);
            if (ctor is not null)
            {
                var font = ctor.Invoke(null);
                if (font is not null)
                {
                    LogSource?.LogInfo("[Font] Font() default constructor succeeded (limited use)");
                    return font;
                }
            }
        }
        catch (Exception ex)
        {
            LogSource?.LogInfo($"[Font] Font() default constructor failed: {ex.InnerException?.Message ?? ex.Message}");
        }

        return null;
    }

    private static object? TryCreateTmpFontAsset(Type tmpFontType, Type fontType, object unityFont)
    {
        // Strategy 1: CreateFontAsset(Font)
        try
        {
            var createFA = tmpFontType.GetMethod("CreateFontAsset",
                BindingFlags.Public | BindingFlags.Static, null, new[] { fontType }, null);
            if (createFA is not null)
            {
                var result = createFA.Invoke(null, new[] { unityFont });
                if (result is not null)
                {
                    LogSource?.LogInfo("[Font] TMP_FontAsset.CreateFontAsset(Font) succeeded");
                    return result;
                }
            }
        }
        catch (Exception ex)
        {
            LogSource?.LogInfo($"[Font] CreateFontAsset(Font) failed: {ex.InnerException?.Message ?? ex.Message}");
        }

        // Strategy 2: Multi-param overloads
        try
        {
            var methods = tmpFontType.GetMethods(BindingFlags.Public | BindingFlags.Static)
                .Where(m => m.Name == "CreateFontAsset" && m.GetParameters().Length >= 2)
                .OrderBy(m => m.GetParameters().Length)
                .ToArray();

            foreach (var method in methods)
            {
                try
                {
                    var pars = method.GetParameters();
                    var args = new object[pars.Length];
                    args[0] = unityFont;
                    if (pars.Length > 1) args[1] = 36;
                    if (pars.Length > 2) args[2] = 4;
                    if (pars.Length > 3)
                    {
                        var renderModeType = pars[3].ParameterType;
                        args[3] = Enum.ToObject(renderModeType, 4134); // SDFAA
                    }
                    if (pars.Length > 4) args[4] = 4096;
                    if (pars.Length > 5) args[5] = 4096;
                    for (var i = 6; i < pars.Length; i++)
                    {
                        args[i] = pars[i].HasDefaultValue ? pars[i].DefaultValue! :
                            (pars[i].ParameterType.IsValueType ? Activator.CreateInstance(pars[i].ParameterType)! : null!);
                    }

                    var result = method.Invoke(null, args);
                    if (result is not null)
                    {
                        LogSource?.LogInfo($"[Font] CreateFontAsset({pars.Length} params) succeeded");
                        return result;
                    }
                }
                catch (Exception ex)
                {
                    LogSource?.LogInfo($"[Font] CreateFontAsset({method.GetParameters().Length} params) failed: {ex.InnerException?.Message ?? ex.Message}");
                }
            }
        }
        catch (Exception ex)
        {
            LogSource?.LogInfo($"[Font] Multi-param CreateFontAsset search failed: {ex.Message}");
        }

        return null;
    }

    private static void TryInjectFallbackFont(object? instance)
    {
        if (_koreanFontAsset is null || instance is null) return;

        try
        {
            var fontProp = instance.GetType().GetProperty("font",
                BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
            var currentFontAsset = fontProp?.GetValue(instance);
            if (currentFontAsset is null) return;

            var instanceId = GetObjectInstanceId(currentFontAsset);
            lock (StateLock)
            {
                if (!_fontInjectedAssets.Add(instanceId)) return;
            }

            var fallbackProp = currentFontAsset.GetType().GetProperty("fallbackFontAssetTable",
                BindingFlags.Instance | BindingFlags.Public);
            if (fallbackProp is null) return;

            var fallbacks = fallbackProp.GetValue(currentFontAsset);
            if (fallbacks is null)
            {
                var listType = fallbackProp.PropertyType;
                fallbacks = Activator.CreateInstance(listType);
                if (fallbacks is null) return;
                fallbackProp.SetValue(currentFontAsset, fallbacks);
            }

            // Check if already in list
            var countProp = fallbacks.GetType().GetProperty("Count");
            var itemProp = fallbacks.GetType().GetProperty("Item");
            if (countProp is not null && itemProp is not null)
            {
                var count = (int)(countProp.GetValue(fallbacks) ?? 0);
                for (var i = 0; i < count; i++)
                {
                    if (ReferenceEquals(itemProp.GetValue(fallbacks, new object[] { i }), _koreanFontAsset))
                        return;
                }
            }

            var addMethod = fallbacks.GetType().GetMethod("Add");
            addMethod?.Invoke(fallbacks, new[] { _koreanFontAsset });
        }
        catch (Exception ex)
        {
            LogSource?.LogWarning($"[Font] Fallback injection failed: {ex.Message}");
        }
    }

    // =========================================================================
    // TryTranslate Chain — 4 Stages (D-05 revised)
    // =========================================================================

    /// <summary>
    /// 4-stage translation chain per D-05 (revised):
    /// 1. TranslationMap (exact match)
    /// 2. DC/FC prefix strip -> TranslationMap lookup
    /// 3. Contextual (source_file + history scoring)
    /// 4. RuntimeLexicon (exact + substring + regex — includes GeneratedPattern)
    /// </summary>
    internal static bool TryTranslate(ref string value, string origin = "unknown")
    {
        if (string.IsNullOrEmpty(value)) return false;
        var original = value;

        // Periodic flush every 500 attempts
        var attempts = Interlocked.Increment(ref _totalTranslateAttempts);
        if (attempts - _lastFlushAttempt >= 500)
        {
            _lastFlushAttempt = attempts;
            try
            {
                WriteUntranslatedCapture();
                WriteState("periodic");
            }
            catch { /* best-effort */ }
        }

        // Stage 1: Exact dictionary lookup (TranslationMap)
        if (TranslationMap.TryGetValue(value, out var t) && !string.IsNullOrEmpty(t))
        {
            value = t;
            RecordHit("exact", original, t);
            Interlocked.Increment(ref _translationMapHits);
            return true;
        }

        // Stage 2: DC/FC prefix strip -> lookup body in TranslationMap
        if (TryTranslateDcFcStrip(ref value, original))
        {
            RecordHit("dcfc_strip", original, value);
            Interlocked.Increment(ref _dcfcStripHits);
            return true;
        }

        // Stage 3: Contextual (source_file + history scoring)
        if (TryTranslateContextual(ref value, original))
        {
            RecordHit("contextual", original, value);
            Interlocked.Increment(ref _contextualHits);
            return true;
        }

        // Stage 4: Runtime lexicon (exact + substring + regex)
        if (TryTranslateRuntimeLexicon(ref value))
        {
            RecordHit("lexicon", original, value);
            Interlocked.Increment(ref _runtimeLexiconHits);
            return true;
        }

        Interlocked.Increment(ref _misses);
        CaptureUntranslated(original, origin);
        return false;
    }

    private static bool TryTranslateDcFcStrip(ref string value, string original)
    {
        var match = DcFcPrefixRegex.Match(value);
        if (!match.Success) return false;

        var body = value.Substring(match.Length);
        if (string.IsNullOrWhiteSpace(body)) return false;

        if (TranslationMap.TryGetValue(body, out var t) && !string.IsNullOrEmpty(t))
        {
            value = t;
            return true;
        }

        return false;
    }

    /// <summary>
    /// Contextual translation: disambiguate same-source-text entries using source_file and history.
    /// Simplified from v1: no NormalizeKey, no StripGameplayPrefix — v2 sources are already clean.
    /// </summary>
    private static bool TryTranslateContextual(ref string value, string original)
    {
        if (!ContextualMap.TryGetValue(value, out var candidates) || candidates.Count == 0)
        {
            return false;
        }

        // Single candidate: use directly
        if (candidates.Count == 1)
        {
            value = candidates[0].Target;
            return true;
        }

        // Multiple candidates: score by _currentDialogSourceFile match
        var currentSourceFile = _currentDialogSourceFile.Trim();
        ContextualEntry? best = null;
        var bestScore = 0;
        var tied = false;

        foreach (var candidate in candidates)
        {
            var score = 0;

            // Source file match is highest priority
            if (!string.IsNullOrEmpty(currentSourceFile) &&
                string.Equals(candidate.SourceFile.Trim(), currentSourceFile, StringComparison.Ordinal))
            {
                score += 8;
            }

            // Recent context history overlap
            if (_recentContextHistory.Count > 0 && !string.IsNullOrEmpty(candidate.SourceFile))
            {
                if (_recentContextHistory.Contains(candidate.SourceFile))
                    score += 4;
            }

            // Speaker hint presence
            if (!string.IsNullOrEmpty(candidate.SpeakerHint))
                score += 1;

            // Dialogue/choice role bonus
            if (string.Equals(candidate.TextRole, "dialogue", StringComparison.OrdinalIgnoreCase) ||
                string.Equals(candidate.TextRole, "choice", StringComparison.OrdinalIgnoreCase))
            {
                score += 1;
            }

            if (score > bestScore)
            {
                best = candidate;
                bestScore = score;
                tied = false;
            }
            else if (score == bestScore && best is not null &&
                     !string.Equals(best.Target, candidate.Target, StringComparison.Ordinal))
            {
                tied = true;
            }
        }

        if (best is null || tied) return false;

        value = best.Target;
        return true;
    }

    /// <summary>
    /// Runtime lexicon: exact -> substring (longest-match-first) -> regex.
    /// Includes GeneratedPattern (ability scores) via regex_rules.
    /// </summary>
    private static bool TryTranslateRuntimeLexicon(ref string value)
    {
        // Exact replacement
        if (RuntimeExactReplacements.TryGetValue(value, out var exactReplacement))
        {
            value = exactReplacement;
            return true;
        }

        // Substring replacements (longest-match-first)
        var modified = false;
        foreach (var pair in RuntimeSubstringReplacements)
        {
            if (value.Contains(pair.Key, StringComparison.Ordinal))
            {
                value = value.Replace(pair.Key, pair.Value, StringComparison.Ordinal);
                modified = true;
            }
        }

        if (modified) return true;

        // Regex rules
        foreach (var rule in RuntimeRegexRules)
        {
            if (rule.Pattern.IsMatch(value))
            {
                value = rule.Pattern.Replace(value, rule.ReplaceString);
                return true;
            }
        }

        return false;
    }

    // =========================================================================
    // Patch Handlers
    // =========================================================================

    private static void TmpTextPrefix(ref string value)
    {
        if (!EnterPatchedCall()) return;

        try
        {
            if (TryTranslate(ref value, "tmp_text"))
            {
                value = CleanOrphanBoldTags(value);
            }
        }
        finally
        {
            ExitPatchedCall();
        }
    }

    private static void TextAssetTextPostfix(object? __instance, ref string __result)
    {
        if (__instance is null || string.IsNullOrEmpty(__result) || TextAssetOverrides.Count == 0)
        {
            return;
        }

        var assetName = ExtractUnityObjectName(__instance);
        if (string.IsNullOrWhiteSpace(assetName))
        {
            return;
        }

        if (TextAssetOverrides.TryGetValue(assetName, out var replacement) && !string.IsNullOrEmpty(replacement))
        {
            __result = replacement;
            Interlocked.Increment(ref _textAssetOverrideHits);
            return;
        }

        lock (StateLock)
        {
            if (_textAssetOverrideMissLoggedCount < 40 && TextAssetOverrideMissSeen.Add(assetName))
            {
                _textAssetOverrideMissLoggedCount++;
                LogSource?.LogInfo($"TextAsset override miss: {assetName}");
            }
        }
    }

    /// <summary>
    /// Game sends choice text pre-wrapped with TMP tags:
    ///   &lt;#FFFFFFFF&gt;&lt;line-indent=-10%&gt;&lt;link="N"&gt;N.   BODY&lt;/link&gt;&lt;/line-indent&gt;&lt;/color&gt;
    /// Extract body, translate, re-wrap.
    /// </summary>
    private static void DialogAddChoiceTextPrefix(object? __instance, ref string text)
    {
        if (string.IsNullOrWhiteSpace(text)) return;

        var match = ChoiceWrapperRegex.Match(text);
        if (match.Success)
        {
            var prefix = match.Groups[1].Value;  // tags + "N.   "
            var body = match.Groups[2].Value;     // choice body text
            var suffix = match.Groups[3].Value;   // </link></line-indent></color>

            if (!string.IsNullOrWhiteSpace(body))
            {
                var originalBody = body;
                TryTranslate(ref body, "ink_choice");
                if (body != originalBody)
                {
                    text = prefix + body + suffix;
                }
            }
        }
        else
        {
            // Fallback: no wrapper detected, try direct translation
            TryTranslate(ref text, "ink_choice");
        }
    }

    /// <summary>
    /// Sets _currentDialogSourceFile for ContextualMap disambiguation.
    /// </summary>
    private static void DialogStartDialogPrefix(object? inkAsset)
    {
        _currentDialogSourceFile = ExtractUnityObjectName(inkAsset);
        _recentContextHistory.Clear();
    }

    private static void LocalizeCheckLanguagePostfix(string ID, ref string __result)
    {
        if (string.IsNullOrWhiteSpace(ID) || LocalizationIdOverrides.Count == 0) return;

        if (LocalizationIdOverrides.TryGetValue(ID, out var replacement) && !string.IsNullOrWhiteSpace(replacement))
        {
            __result = replacement;
            Interlocked.Increment(ref _localizationIdOverrideHits);
        }
    }

    private static void TmpOnEnablePostfix(object? __instance)
    {
        TryLoadKoreanFont();
        TryInjectFallbackFont(__instance);
        TranslateCurrentTextProperty(__instance);
    }

    private static void TmpConcreteAwakePostfix(object? __instance)
    {
        TryLoadKoreanFont();
        TryInjectFallbackFont(__instance);
        TranslateCurrentTextProperty(__instance);
    }

    private static void UiTextPrefix(ref string value)
    {
        if (!EnterPatchedCall()) return;

        try
        {
            TryTranslate(ref value, "ui_text");
        }
        finally
        {
            ExitPatchedCall();
        }
    }

    private static void UiOnEnablePostfix(object? __instance)
    {
        TranslateCurrentTextProperty(__instance);
    }

    private static void MenuControllerStartPostfix(object? __instance)
    {
        TranslateActiveSceneUI();
    }

    private static void MenuControllerRefreshPostfix(object? __instance)
    {
        TranslateActiveSceneUI();
    }

    private static void SceneLoadedPostfix(object? scene, object? mode)
    {
        // Flush capture buffers on scene transition
        WriteUntranslatedCapture();
        WriteTranslationHits();
        WriteState("scene_loaded");
    }

    // =========================================================================
    // Helpers — Text Property Translation
    // =========================================================================

    private static void TranslateCurrentTextProperty(object? instance)
    {
        if (instance is null || !EnterPatchedCall()) return;

        try
        {
            var prop = instance.GetType().GetProperty("text",
                BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
            if (prop is null || !prop.CanRead || !prop.CanWrite) return;

            if (prop.GetValue(instance) is not string currentText || string.IsNullOrWhiteSpace(currentText))
                return;

            var translated = currentText;
            if (!TryTranslate(ref translated, "property_scan") ||
                string.Equals(translated, currentText, StringComparison.Ordinal))
                return;

            translated = CleanOrphanBoldTags(translated);
            prop.SetValue(instance, translated);
        }
        catch { }
        finally
        {
            ExitPatchedCall();
        }
    }

    /// <summary>
    /// Simplified scene sweep: find all TMP_Text in active scene, call TryTranslate on each.
    /// </summary>
    private static void TranslateActiveSceneUI()
    {
        try
        {
            var resourcesType = FindTypeByName("UnityEngine.Resources");
            var tmpTextType = FindTypeByName("TMPro.TMP_Text");
            if (resourcesType is null || tmpTextType is null) return;

            var findAll = resourcesType.GetMethods(BindingFlags.Public | BindingFlags.Static)
                .FirstOrDefault(m =>
                {
                    if (!string.Equals(m.Name, "FindObjectsOfTypeAll", StringComparison.Ordinal)) return false;
                    var parameters = m.GetParameters();
                    return parameters.Length == 1 && parameters[0].ParameterType == typeof(Type);
                });
            if (findAll is null) return;

            if (findAll.Invoke(null, new object[] { tmpTextType }) is not System.Collections.IEnumerable objects) return;

            foreach (var tmp in objects)
            {
                TranslateCurrentTextProperty(tmp);
            }
        }
        catch (Exception ex)
        {
            LogSource?.LogWarning($"Scene text sweep failed: {ex.Message}");
        }
    }

    // =========================================================================
    // Capture / Debug Infrastructure (D-12, D-13)
    // =========================================================================

    private static void CaptureUntranslated(string text, string origin)
    {
        if (string.IsNullOrWhiteSpace(text)) return;

        // Basic filter: skip very short strings and non-English text
        if (text.Length < 2) return;

        lock (StateLock)
        {
            _untranslatedCapture.Add(text);
        }
    }

    private static void RecordHit(string stage, string source, string target)
    {
        // Update context history
        if (!string.IsNullOrEmpty(_currentDialogSourceFile))
        {
            _recentContextHistory.Add(_currentDialogSourceFile);
            if (_recentContextHistory.Count > RecentHistoryLimit)
            {
                // HashSet doesn't support index removal — clear and rebuild if over limit
                // This is a simplification; in practice the set grows slowly
            }
        }

        if (!_fullCaptureEnabled) return;

        lock (StateLock)
        {
            _translationHitsCapture[source] = target;
        }
    }

    private static void WriteUntranslatedCapture()
    {
        if (string.IsNullOrWhiteSpace(_capturePath)) return;

        try
        {
            string[] items;
            lock (StateLock)
            {
                items = _untranslatedCapture.OrderBy(x => x, StringComparer.Ordinal).ToArray();
            }

            if (items.Length == 0) return;

            var entries = items.Select(s => new
            {
                source = s,
                target = "",
                status = "new",
                category = "runtime_capture",
                source_file = "runtime",
                tags = new[] { "runtime_missing" }
            }).ToArray();

            var payload = new
            {
                generated_at = DateTime.Now.ToString("s"),
                count = entries.Length,
                entries
            };

            File.WriteAllText(_capturePath, JsonSerializer.Serialize(payload, new JsonSerializerOptions
            {
                WriteIndented = true
            }));
        }
        catch (Exception ex)
        {
            LogSource?.LogWarning($"Failed to write untranslated capture: {ex.Message}");
        }
    }

    private static void WriteTranslationHits()
    {
        if (!_fullCaptureEnabled || string.IsNullOrWhiteSpace(_translationHitLogPath)) return;

        try
        {
            KeyValuePair<string, string>[] items;
            lock (StateLock)
            {
                items = _translationHitsCapture.ToArray();
            }

            if (items.Length == 0) return;

            var entries = items.Select(kv => new { source = kv.Key, target = kv.Value }).ToArray();
            var payload = new
            {
                generated_at = DateTime.Now.ToString("s"),
                count = entries.Length,
                entries
            };

            File.WriteAllText(_translationHitLogPath, JsonSerializer.Serialize(payload, new JsonSerializerOptions
            {
                WriteIndented = true
            }), System.Text.Encoding.UTF8);
        }
        catch { }
    }

    private static void WriteState(string phase)
    {
        if (string.IsNullOrWhiteSpace(_statePath)) return;

        try
        {
            int untranslatedCount;
            lock (StateLock)
            {
                untranslatedCount = _untranslatedCapture.Count;
            }

            var contextualTotal = 0;
            foreach (var list in ContextualMap.Values)
                contextualTotal += list.Count;

            var payload = new
            {
                phase,
                plugin_version = PluginVersion,
                written_at = DateTime.Now.ToString("s"),
                translations_loaded = TranslationMap.Count,
                contextual_loaded = contextualTotal,
                text_asset_overrides = TextAssetOverrides.Count,
                text_asset_override_hits = _textAssetOverrideHits,
                localization_overrides = LocalizationIdOverrides.Count,
                localization_override_hits = _localizationIdOverrideHits,
                lexicon_exact = RuntimeExactReplacements.Count,
                lexicon_substrings = RuntimeSubstringReplacements.Count,
                lexicon_regex = RuntimeRegexRules.Count,
                hits_exact = _translationMapHits,
                hits_dcfc_strip = _dcfcStripHits,
                hits_contextual = _contextualHits,
                hits_lexicon = _runtimeLexiconHits,
                total_misses = _misses,
                untranslated_count = untranslatedCount,
                font_status = _fontStatus
            };

            File.WriteAllText(_statePath, JsonSerializer.Serialize(payload, new JsonSerializerOptions
            {
                WriteIndented = true
            }));
        }
        catch (Exception ex)
        {
            LogSource?.LogWarning($"Failed to write state: {ex.Message}");
        }
    }

    // =========================================================================
    // Helpers — General
    // =========================================================================

    private static bool EnterPatchedCall()
    {
        if (_patchReentryDepth > 0) return false;
        _patchReentryDepth++;
        return true;
    }

    private static void ExitPatchedCall()
    {
        if (_patchReentryDepth > 0) _patchReentryDepth--;
    }

    /// <summary>
    /// Per D-06: Only strip unmatched bold tags. Balanced pairs are preserved.
    /// Fix for Phase 4 Bug 4 — v2 translations contain intentional bold tags.
    /// </summary>
    private static string CleanOrphanBoldTags(string text)
    {
        if (string.IsNullOrEmpty(text)) return text;

        int opens = 0, closes = 0;
        int idx = 0;
        while ((idx = text.IndexOf("<b>", idx, StringComparison.Ordinal)) >= 0) { opens++; idx += 3; }
        idx = 0;
        while ((idx = text.IndexOf("</b>", idx, StringComparison.Ordinal)) >= 0) { closes++; idx += 4; }

        if (opens == closes) return text; // balanced — keep

        // Unbalanced: strip all (safety)
        return text.Replace("<b>", "").Replace("</b>", "");
    }

    private static List<string> ParseCsvLine(string line)
    {
        var values = new List<string>();
        var current = new System.Text.StringBuilder();
        var inQuotes = false;
        for (var i = 0; i < line.Length; i++)
        {
            var ch = line[i];
            if (ch == '"')
            {
                if (inQuotes && i + 1 < line.Length && line[i + 1] == '"')
                {
                    current.Append('"');
                    i++;
                }
                else
                {
                    inQuotes = !inQuotes;
                }
                continue;
            }

            if (ch == ',' && !inQuotes)
            {
                values.Add(current.ToString());
                current.Clear();
                continue;
            }

            current.Append(ch);
        }
        values.Add(current.ToString());
        return values;
    }

    private static string? ReadString(JsonElement element, string propertyName)
    {
        if (!element.TryGetProperty(propertyName, out var value) || value.ValueKind != JsonValueKind.String)
        {
            return null;
        }
        return value.GetString();
    }

    private static Type? FindTypeByName(string name)
    {
        lock (TypeCache)
        {
            if (TypeCache.TryGetValue(name, out var cached))
                return cached;
        }

        foreach (var asm in AppDomain.CurrentDomain.GetAssemblies())
        {
            var type = asm.GetType(name, throwOnError: false);
            if (type is not null)
            {
                lock (TypeCache) { TypeCache[name] = type; }
                return type;
            }
        }

        return null;
    }

    private static Type? FindTypeInAssemblies(string[] assemblyCandidates, string typeName)
    {
        foreach (var asmName in assemblyCandidates)
        {
            var asm = AppDomain.CurrentDomain.GetAssemblies()
                .FirstOrDefault(a => string.Equals(a.GetName().Name, asmName, StringComparison.Ordinal));
            var type = asm?.GetType(typeName, throwOnError: false);
            if (type is not null)
            {
                return type;
            }
        }

        return null;
    }

    private static string ExtractUnityObjectName(object? value)
    {
        if (value is null) return "";

        try
        {
            var property = value.GetType().GetProperty("name",
                BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
            if (property?.GetValue(value) is string name && !string.IsNullOrWhiteSpace(name))
            {
                return name.Trim();
            }
        }
        catch { }

        return "";
    }

    private static int GetObjectInstanceId(object unityObject)
    {
        try
        {
            var method = unityObject.GetType().GetMethod("GetInstanceID",
                BindingFlags.Instance | BindingFlags.Public);
            if (method?.Invoke(unityObject, null) is int id) return id;
        }
        catch { }

        return RuntimeHelpers.GetHashCode(unityObject);
    }

    private static void SetObjectHideFlags(object unityObject, int flags)
    {
        try
        {
            var prop = unityObject.GetType().GetProperty("hideFlags",
                BindingFlags.Instance | BindingFlags.Public);
            if (prop is not null)
            {
                var hideFlagsType = prop.PropertyType;
                prop.SetValue(unityObject, Enum.ToObject(hideFlagsType, flags));
            }
        }
        catch { }
    }

    // =========================================================================
    // Inner Types
    // =========================================================================

    private class ContextualEntry
    {
        public string Source { get; set; } = "";
        public string Target { get; set; } = "";
        public string SourceFile { get; set; } = "";
        public string TextRole { get; set; } = "";
        public string SpeakerHint { get; set; } = "";
    }

    private class RuntimeRegexRule
    {
        public string Name { get; set; } = "";
        public Regex Pattern { get; set; } = null!;
        public string ReplaceString { get; set; } = "";
    }
}
