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

[BepInPlugin(PluginGuid, PluginName, PluginVersion)]
public class Plugin : BasePlugin
{
    public const string PluginGuid = "com.esotericebb.translationloader";
    public const string PluginName = "EsotericEbb Translation Loader";
    public const string PluginVersion = "0.3.0";

    internal static ManualLogSource? LogSource;
    internal static readonly Dictionary<string, string> TranslationMap = new(StringComparer.Ordinal);
    internal static readonly Dictionary<string, string> NormalizedMap = new(StringComparer.Ordinal);
    private static readonly Dictionary<string, List<ContextualEntry>> ContextualMap = new(StringComparer.Ordinal);
    private static readonly Dictionary<string, string> TextAssetOverrides = new(StringComparer.OrdinalIgnoreCase);
    private static readonly Dictionary<string, string> LocalizationIdOverrides = new(StringComparer.OrdinalIgnoreCase);
    private static readonly List<KeyValuePair<string, string>> RuntimeSubstringReplacements = new();
    private static readonly List<RuntimeRegexRule> RuntimeRegexRules = new();
    private static readonly Dictionary<string, string> MenuDirectOverrides = new(StringComparer.Ordinal)
    {
        ["New Game"] = "새 게임",
        ["Load"] = "불러오기",
        ["Options"] = "옵션",
        ["Credits"] = "크레딧",
        ["Exit"] = "종료",
        ["Load Game"] = "게임 불러오기",
        ["Load file?"] = "파일을 불러올까요?",
        ["- Load File -"] = "- 파일 불러오기 -"
    };
    private static readonly Dictionary<string, string> StatNameOverrides = new(StringComparer.Ordinal)
    {
        ["Strength"] = "힘",
        ["Dexterity"] = "민첩",
        ["Constitution"] = "건강",
        ["Intelligence"] = "지능",
        ["Wisdom"] = "지혜",
        ["Charisma"] = "매력"
    };
    private static readonly HashSet<string> TextAssetOverrideMissSeen = new(StringComparer.OrdinalIgnoreCase);
    private static readonly HashSet<string> LocalizationMissSeen = new(StringComparer.OrdinalIgnoreCase);

    private static readonly HashSet<string> UntranslatedSeen = new(StringComparer.Ordinal);
    private static readonly HashSet<string> ChoiceCaptureSeen = new(StringComparer.Ordinal);
    private static readonly List<object> FullCaptureBuffer = new();
    private static readonly HashSet<string> FullCaptureSeen = new(StringComparer.Ordinal);
    private static readonly HashSet<string> DumpedMenuScenes = new(StringComparer.Ordinal);
    private static readonly HashSet<string> DumpedUiToolkitEntries = new(StringComparer.Ordinal);
    private static readonly HashSet<string> TriggeredSceneScans = new(StringComparer.Ordinal);
    private static bool _choiceSignatureLogged;
    private static readonly List<string> RecentNormalizedHistory = new();
    private static readonly object StateLock = new();
    private static readonly object ContextLock = new();
    private static readonly Dictionary<string, Type?> TypeCache = new(StringComparer.Ordinal);
    private const int RecentHistoryLimit = 8;
    [ThreadStatic] private static int _patchReentryDepth;
    [ThreadStatic] private static bool _suppressDiagnostics;
    private static string? _capturePath;
    private static string? _choiceCapturePath;
    private static string? _fullCapturePath;
    private static bool _fullCaptureEnabled;
    private static int _fullCaptureFlushCount;
    private static string? _translationHitLogPath;
    private static readonly List<object> _translationHitLog = new();
    private static int _translationHitLogFlushCount;
    private static string? _statePath;
    private static string? _menuDumpPath;
    private static string? _uiToolkitDumpPath;
    private static int _newCaptureCount;
    private static int _choiceCaptureCount;
    private static int _choiceInternalBranchCount;
    private static int _choiceTemplateCount;
    private static int _choiceStatGateCount;
    private static int _choiceShortResultCount;
    private static int _choiceNormalCount;
    private static int _translateHitCount;
    private static int _translateMissCount;
    private static int _contextualLoadedCount;
    private static int _textAssetOverrideCount;
    private static int _textAssetOverrideHitCount;
    private static int _textAssetOverrideMissLoggedCount;
    private static int _localizationIdOverrideCount;
    private static int _localizationIdOverrideHitCount;
    private static int _localizationIdOverrideMissLoggedCount;
    private static int _runtimeLexiconSubstringCount;
    private static int _runtimeLexiconRegexCount;
    private static int _runtimeLexiconHitCount;
    private static int _menuSceneSweepCount;
    private static int _menuSceneTranslatedCount;
    private static int _menuSweepSampleCount;
    private static int _menuDirectOverrideHits;
    private static int _sceneTextScanCount;
    private static int _contextualHitCount;
    private static int _sourceFileContextHitCount;
    private static string _lastTranslatedSource = string.Empty;
    private static string _lastTranslatedTarget = string.Empty;
    private static string _lastMissedSource = string.Empty;
    [ThreadStatic] private static string? _currentDialogSourceFile;

    // Font injection state
    private static object? _koreanFontAsset;
    private static bool _fontLoadAttempted;
    private static readonly HashSet<int> _fontInjectedAssets = new();
    private static int _fontFallbackInjectedCount;
    private static string _fontStatus = "not_attempted";

    [DllImport("gdi32.dll", CharSet = CharSet.Unicode)]
    private static extern int AddFontResourceEx(string lpFileName, uint fl, IntPtr pdv);

    private const uint FR_PRIVATE = 0x10;
    private const string KoreanFontFamily = "YeolrinMyeongjo";

    private Harmony? _harmony;
    private bool _tmpSetterPatched;
    private bool _tmpEnablePatched;
    private bool _tmpUguiAwakePatched;
    private bool _tmpWorldAwakePatched;
    private bool _tmpPatched;
    private bool _textAssetPatched;
    private bool _localizationManagerPatched;
    private bool _uiEnablePatched;
    private bool _uiPatched;
    private bool _uiToolkitPatched;
    private bool _sceneManagerPatched;
    private bool _menuPatched;
    private bool _dialogPatched;

    public override void Load()
    {
        LogSource = Log;
        _capturePath = Path.Combine(Paths.GameRootPath, "BepInEx", "untranslated_capture.json");
        _choiceCapturePath = Path.Combine(Paths.GameRootPath, "BepInEx", "choice_capture.json");
        _fullCapturePath = Path.Combine(Paths.GameRootPath, "BepInEx", "full_text_capture.json");
        _translationHitLogPath = Path.Combine(Paths.GameRootPath, "BepInEx", "translation_hits.json");
        _fullCaptureEnabled = File.Exists(Path.Combine(Paths.GameRootPath, "BepInEx", "ENABLE_FULL_CAPTURE"));
        _statePath = Path.Combine(Paths.GameRootPath, "BepInEx", "translation_loader_state.json");
        _menuDumpPath = Path.Combine(Paths.GameRootPath, "BepInEx", "menu_runtime_dump.json");
        _uiToolkitDumpPath = Path.Combine(Paths.GameRootPath, "BepInEx", "ui_toolkit_runtime_dump.txt");
        WriteState("load_start");
        LoadTranslations();
        LoadTextAssetOverrides();
        LoadLocalizationIdOverrides();
        LoadRuntimeLexicon();

        _harmony = new Harmony(PluginGuid);
        TryApplyPatches("initial");

        AppDomain.CurrentDomain.AssemblyLoad += OnAssemblyLoad;

        Log.LogInfo($"{PluginName} loaded. entries={TranslationMap.Count}, contextual={_contextualLoadedCount}");
        if (_fullCaptureEnabled)
        {
            Log.LogInfo("FULL CAPTURE MODE enabled. All text will be logged to full_text_capture.json");
        }
        WriteState("load_complete");
    }

    private void OnAssemblyLoad(object? sender, AssemblyLoadEventArgs args)
    {
        if (_tmpPatched && _uiPatched && _dialogPatched)
        {
            return;
        }

        var name = args.LoadedAssembly.GetName().Name ?? string.Empty;
        if (name.Contains("TextMeshPro", StringComparison.OrdinalIgnoreCase) ||
            name.Equals("TMPro", StringComparison.OrdinalIgnoreCase) ||
            name.Equals("UnityEngine.UI", StringComparison.OrdinalIgnoreCase) ||
            name.Equals("Assembly-CSharp", StringComparison.OrdinalIgnoreCase))
        {
            TryApplyPatches($"assembly-load:{name}");
        }
    }

    private void TryApplyPatches(string reason)
    {
        if (_harmony is null)
        {
            return;
        }

        if (!_tmpSetterPatched)
        {
            _tmpSetterPatched = ApplyTextPatch(_harmony,
                new[] { "TMPro", "Unity.TextMeshPro" },
                "TMPro.TMP_Text",
                "text",
                nameof(TmpTextPrefix));
        }

        if (!_tmpEnablePatched)
        {
            _tmpEnablePatched = ApplyMethodPatch(_harmony,
                new[] { "TMPro", "Unity.TextMeshPro" },
                "TMPro.TMP_Text",
                "OnEnable",
                nameof(TmpOnEnablePostfix),
                parameterCount: 0,
                usePostfix: true);
        }

        if (!_tmpUguiAwakePatched)
        {
            _tmpUguiAwakePatched = ApplyMethodPatch(_harmony,
                new[] { "TMPro", "Unity.TextMeshPro" },
                "TMPro.TextMeshProUGUI",
                "Awake",
                nameof(TmpConcreteAwakePostfix),
                parameterCount: 0,
                usePostfix: true);
        }

        if (!_tmpWorldAwakePatched)
        {
            _tmpWorldAwakePatched = ApplyMethodPatch(_harmony,
                new[] { "TMPro", "Unity.TextMeshPro" },
                "TMPro.TextMeshPro",
                "Awake",
                nameof(TmpConcreteAwakePostfix),
                parameterCount: 0,
                usePostfix: true);
        }

        _tmpPatched = _tmpSetterPatched &&
            _tmpEnablePatched &&
            _tmpUguiAwakePatched &&
            _tmpWorldAwakePatched;

        if (_tmpPatched)
        {
            TryLoadKoreanFont();
        }

        if (!_textAssetPatched)
        {
            _textAssetPatched = ApplyTextAssetPatch(_harmony);
        }

        if (!_localizationManagerPatched)
        {
            _localizationManagerPatched = ApplyMethodPatch(_harmony,
                new[] { "Assembly-CSharp" },
                "LocalizationManager",
                "CheckLanguage",
                nameof(LocalizeCheckLanguagePostfix),
                parameterCount: 1,
                usePostfix: true);
        }

        if (!_uiPatched)
        {
            _uiPatched = ApplyTextPatch(_harmony,
                new[] { "UnityEngine.UI" },
                "UnityEngine.UI.Text",
                "text",
                nameof(UiTextPrefix));
        }

        if (!_uiEnablePatched)
        {
            _uiEnablePatched = ApplyMethodPatch(_harmony,
                new[] { "UnityEngine.UI" },
                "UnityEngine.UI.Text",
                "OnEnable",
                nameof(UiOnEnablePostfix),
                parameterCount: 0,
                usePostfix: true);
        }

        _uiPatched = _uiPatched && _uiEnablePatched;

        if (!_uiToolkitPatched)
        {
            _uiToolkitPatched = ApplyTextPatch(_harmony,
                new[] { "UnityEngine.UIElementsModule" },
                "UnityEngine.UIElements.TextElement",
                "text",
                nameof(UiElementsTextPrefix));
        }

        if (!_sceneManagerPatched)
        {
            _sceneManagerPatched = ApplyMethodPatch(_harmony,
                new[] { "UnityEngine.CoreModule", "UnityEngine" },
                "UnityEngine.SceneManagement.SceneManager",
                "Internal_SceneLoaded",
                nameof(SceneLoadedPostfix),
                parameterCount: 2,
                usePostfix: true);
        }

        if (!_menuPatched)
        {
            var menuStartPatched = ApplyMethodPatch(_harmony,
                new[] { "Assembly-CSharp" },
                "MenuController",
                "Start",
                nameof(MenuControllerStartPostfix),
                parameterCount: 0,
                usePostfix: true);
            var menuOptionsPatched = ApplyMethodPatch(_harmony,
                new[] { "Assembly-CSharp" },
                "MenuController",
                "Options",
                nameof(MenuControllerRefreshPostfix),
                parameterCount: 0,
                usePostfix: true);
            var menuCreditsPatched = ApplyMethodPatch(_harmony,
                new[] { "Assembly-CSharp" },
                "MenuController",
                "Credits",
                nameof(MenuControllerRefreshPostfix),
                parameterCount: 0,
                usePostfix: true);
            var menuReturnPatched = ApplyMethodPatch(_harmony,
                new[] { "Assembly-CSharp" },
                "MenuController",
                "ReturnToMenu",
                nameof(MenuControllerRefreshPostfix),
                parameterCount: 0,
                usePostfix: true);
            var menuUpdatePatched = ApplyMethodPatch(_harmony,
                new[] { "Assembly-CSharp" },
                "MenuController",
                "Update",
                nameof(MenuControllerUpdatePostfix),
                parameterCount: 0,
                usePostfix: true);
            _menuPatched = menuStartPatched && menuOptionsPatched && menuCreditsPatched && menuReturnPatched && menuUpdatePatched;
        }

        if (!_dialogPatched)
        {
            var startDialogPatched = ApplyMethodPatch(_harmony,
                new[] { "Assembly-CSharp" },
                "DialogManager",
                "StartDialog",
                nameof(DialogStartDialogPrefix),
                parameterCount: 5);

            var addTextPatched = ApplyMethodPatch(_harmony,
                new[] { "Assembly-CSharp" },
                "DialogManager",
                "AddText",
                nameof(DialogAddTextPrefix),
                parameterCount: 1);

            var addChoicePatched = ApplyMethodPatch(_harmony,
                new[] { "Assembly-CSharp" },
                "DialogManager",
                "AddChoiceText",
                nameof(DialogAddChoiceTextPrefix),
                parameterCount: 9);

            _dialogPatched = startDialogPatched && addTextPatched && addChoicePatched;
        }

        InstanceState = new PluginState
        {
            TmpPatched = _tmpPatched,
            TextAssetPatched = _textAssetPatched,
            LocalizationManagerPatched = _localizationManagerPatched,
            UiPatched = _uiPatched,
            MenuPatched = _menuPatched,
            DialogPatched = _dialogPatched
        };

        Log.LogInfo($"Patch status ({reason}): TMP={_tmpPatched}, TextAsset={_textAssetPatched}, Localization={_localizationManagerPatched}, UI={_uiPatched}, UIToolkit={_uiToolkitPatched}, SceneManager={_sceneManagerPatched}, Menu={_menuPatched}, Dialog={_dialogPatched}");
        WriteState($"patch_status:{reason}");
    }

    private static void TryLoadKoreanFont()
    {
        if (_fontLoadAttempted)
        {
            return;
        }

        _fontLoadAttempted = true;

        try
        {
            // Step 1: Find the font file
            var fontPath = FindKoreanFontFile();
            if (fontPath is null)
            {
                _fontStatus = "font_file_not_found";
                LogSource?.LogWarning("[Font] Korean font file not found. Place NexonWarhaven-Bold.ttf in TranslationPatch/fonts/");
                return;
            }

            LogSource?.LogInfo($"[Font] Found font file: {fontPath}");

            // Step 2: Find required types
            var fontType = FindTypeByName("UnityEngine.Font");
            if (fontType is null)
            {
                _fontStatus = "unity_font_type_not_found";
                LogSource?.LogWarning("[Font] UnityEngine.Font type not found");
                return;
            }

            var tmpFontType = FindTypeByName("TMPro.TMP_FontAsset");
            if (tmpFontType is null)
            {
                _fontStatus = "tmp_font_type_not_found";
                LogSource?.LogWarning("[Font] TMPro.TMP_FontAsset type not found");
                return;
            }

            // Step 3: Create Unity Font — try multiple strategies
            object? unityFont = null;

            // Strategy A: Font(string) constructor with font path
            // In IL2CPP, Font(string name) internally resolves the font;
            // passing a full file path works in some Unity versions.
            unityFont = TryCreateFontViaConstructor(fontType, fontPath);

            // Strategy B: Register with Windows then use CreateDynamicFontFromOSFont
            if (unityFont is null)
            {
                var added = AddFontResourceEx(fontPath, 0, IntPtr.Zero); // 0 = system-wide for process
                if (added > 0)
                {
                    LogSource?.LogInfo($"[Font] AddFontResourceEx registered {added} font(s)");

                    // Try multiple candidate names
                    var candidateNames = new[] { "Warhaven Bold", "Warhaven", "NEXON Warhaven Bold", "NEXON Warhaven" };
                    var discoveredName = DiscoverFontName(fontType);
                    if (discoveredName is not null)
                    {
                        candidateNames = new[] { discoveredName }.Concat(candidateNames).Distinct().ToArray();
                    }

                    var createMethod = fontType.GetMethod("CreateDynamicFontFromOSFont",
                        BindingFlags.Public | BindingFlags.Static,
                        null, new[] { typeof(string), typeof(int) }, null);

                    if (createMethod is not null)
                    {
                        foreach (var candidate in candidateNames)
                        {
                            try
                            {
                                unityFont = createMethod.Invoke(null, new object[] { candidate, 24 });
                                if (unityFont is not null)
                                {
                                    LogSource?.LogInfo($"[Font] CreateDynamicFontFromOSFont succeeded with: {candidate}");
                                    break;
                                }
                            }
                            catch (Exception ex)
                            {
                                LogSource?.LogInfo($"[Font] CreateDynamicFontFromOSFont failed for '{candidate}': {ex.InnerException?.Message ?? ex.Message}");
                            }
                        }
                    }
                }
                else
                {
                    LogSource?.LogWarning("[Font] AddFontResourceEx returned 0");
                }
            }

            // Strategy C: Font() default constructor + set name, as last resort
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

            // Set HideFlags.DontUnloadUnusedAsset to prevent GC
            SetObjectHideFlags(unityFont, 52);

            // Step 4: Create TMP_FontAsset
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
            if (!Directory.Exists(dir))
            {
                continue;
            }

            foreach (var pattern in filePatterns)
            {
                var files = Directory.GetFiles(dir, pattern)
                    .Where(f => f.EndsWith(".ttf", StringComparison.OrdinalIgnoreCase) ||
                                f.EndsWith(".otf", StringComparison.OrdinalIgnoreCase))
                    .ToArray();

                if (files.Length > 0)
                {
                    // Prefer Regular weight — Bold causes all Korean text to render heavy
                    var regular = files.FirstOrDefault(f =>
                        f.Contains("Regular", StringComparison.OrdinalIgnoreCase));
                    return regular ?? files[0];
                }
            }
        }

        return null;
    }

    private static string? DiscoverFontName(Type fontType)
    {
        try
        {
            var getNamesMethod = fontType.GetMethod("GetOSInstalledFontNames",
                BindingFlags.Public | BindingFlags.Static);
            if (getNamesMethod?.Invoke(null, null) is not string[] fontNames)
            {
                return null;
            }

            // Look for exact or partial match
            var exact = fontNames.FirstOrDefault(n =>
                n.Contains("Warhaven", StringComparison.OrdinalIgnoreCase));
            if (exact is not null)
            {
                LogSource?.LogInfo($"[Font] Discovered OS font name: {exact}");
                return exact;
            }
        }
        catch (Exception ex)
        {
            LogSource?.LogWarning($"[Font] Font name discovery failed: {ex.Message}");
        }

        return null;
    }

    private static object? TryCreateFontViaConstructor(Type fontType, string fontPath)
    {
        try
        {
            // Try Font(string) constructor — in some Unity/IL2CPP versions this accepts a file path
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
                BindingFlags.Public | BindingFlags.Static,
                null, new[] { fontType }, null);
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
                    // Fill remaining with defaults
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
        catch
        {
        }
    }

    private static void TryInjectFallbackFont(object? instance)
    {
        if (_koreanFontAsset is null || instance is null)
        {
            return;
        }

        try
        {
            // Get the TMP component's font asset
            var fontProp = instance.GetType().GetProperty("font",
                BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
            var currentFontAsset = fontProp?.GetValue(instance);
            if (currentFontAsset is null)
            {
                return;
            }

            // Track by instance ID to avoid duplicate injection
            var instanceId = GetObjectInstanceId(currentFontAsset);
            lock (StateLock)
            {
                if (!_fontInjectedAssets.Add(instanceId))
                {
                    return;
                }
            }

            // Get or create fallbackFontAssetTable
            var fallbackProp = currentFontAsset.GetType().GetProperty("fallbackFontAssetTable",
                BindingFlags.Instance | BindingFlags.Public);
            if (fallbackProp is null)
            {
                return;
            }

            var fallbacks = fallbackProp.GetValue(currentFontAsset);
            if (fallbacks is null)
            {
                // Create a new list via the property's type
                var listType = fallbackProp.PropertyType;
                fallbacks = Activator.CreateInstance(listType);
                if (fallbacks is null)
                {
                    return;
                }

                fallbackProp.SetValue(currentFontAsset, fallbacks);
            }

            // Check if our font is already in the list
            var countProp = fallbacks.GetType().GetProperty("Count");
            var itemProp = fallbacks.GetType().GetProperty("Item");
            if (countProp is not null && itemProp is not null)
            {
                var count = (int)(countProp.GetValue(fallbacks) ?? 0);
                for (var i = 0; i < count; i++)
                {
                    var existing = itemProp.GetValue(fallbacks, new object[] { i });
                    if (ReferenceEquals(existing, _koreanFontAsset))
                    {
                        return;
                    }
                }
            }

            // Add our Korean font asset as fallback
            var addMethod = fallbacks.GetType().GetMethod("Add");
            addMethod?.Invoke(fallbacks, new[] { _koreanFontAsset });

            lock (StateLock)
            {
                _fontFallbackInjectedCount++;
            }
        }
        catch (Exception ex)
        {
            LogSource?.LogWarning($"[Font] Fallback injection failed: {ex.Message}");
        }
    }

    private static int GetObjectInstanceId(object unityObject)
    {
        try
        {
            var method = unityObject.GetType().GetMethod("GetInstanceID",
                BindingFlags.Instance | BindingFlags.Public);
            if (method?.Invoke(unityObject, null) is int id)
            {
                return id;
            }
        }
        catch
        {
        }

        return RuntimeHelpers.GetHashCode(unityObject);
    }

    private void LoadTranslations()
    {
        var candidates = new[]
        {
            Path.Combine(Paths.GameRootPath, "Esoteric Ebb_Data", "StreamingAssets", "TranslationPatch", "translations.json"),
            Path.Combine(Paths.GameRootPath, "StreamingAssets", "TranslationPatch", "translations.json")
        };

        var path = candidates.FirstOrDefault(File.Exists);
        if (path is null)
        {
            Log.LogWarning("translations.json not found. loader running with empty map.");
            return;
        }

        try
        {
            var json = File.ReadAllText(path);
            LoadEntriesFromJson(json);
            Log.LogInfo($"Loaded translation file: {path}");
        }
        catch (Exception ex)
        {
            Log.LogError($"Failed to load translations.json: {ex}");
        }
    }

    internal static bool TryTranslate(ref string value, string origin = "unknown")
    {
        if (string.IsNullOrEmpty(value))
        {
            return false;
        }

        CaptureAllText(value, origin);

        var originalValue = value;
        bool found = false;

        // Stage 1: Generated patterns (dynamic UI)
        if (!found && TryTranslateGeneratedPattern(ref value))
        {
            found = true;
        }

        // Stage 2: Direct dictionary lookup
        if (!found && TranslationMap.TryGetValue(value, out var translated) && !string.IsNullOrEmpty(translated))
        {
            translated = RestoreChoicePrefix(originalValue, translated);
            RecordTranslationHit(originalValue, translated);
            value = translated;
            found = true;
        }

        // Stage 2b: DC/FC stat-check prefix stripping
        // Choice text like "DC12 str-The Cleric? I like that." — strip prefix, translate body, reattach
        if (!found)
        {
            var dcfcMatch = System.Text.RegularExpressions.Regex.Match(
                value, @"^([A-Z]{2}\d+\s+\w+-)(.+)$");
            if (dcfcMatch.Success)
            {
                var prefix = dcfcMatch.Groups[1].Value;
                var body = dcfcMatch.Groups[2].Value;
                if (TranslationMap.TryGetValue(body, out var bodyTranslated) && !string.IsNullOrEmpty(bodyTranslated))
                {
                    RecordTranslationHit(originalValue, bodyTranslated);
                    value = prefix + bodyTranslated;
                    found = true;
                }
            }
        }

        // Stage 3: Contextual (disambiguation by source_file + history)
        if (!found)
        {
            var normalized = NormalizeKey(value);
            if (TryTranslateContextual(ref value, originalValue, normalized))
            {
                found = true;
            }
        }

        // Stage 4: Runtime lexicon (substring/regex)
        if (!found && TryTranslateRuntimeLexicon(ref value))
        {
            found = true;
        }

        RememberContext(originalValue);

        if (found)
        {
            value = StripQuotationMarks(value);
            value = CleanOrphanBoldTags(value);
            return true;
        }

        RecordTranslationMiss(originalValue);
        CaptureUntranslated(originalValue);
        return false;
    }

    /// <summary>
    /// Remove straight double-quote characters from translated Korean text.
    /// Game dialogue is already visually framed by the UI, so explicit quotes
    /// look out of place and break immersion.
    /// </summary>
    private static string StripQuotationMarks(string text)
    {
        if (string.IsNullOrEmpty(text) || text.IndexOf('"') < 0)
        {
            return text;
        }
        return text.Replace("\"", "");
    }

    /// <summary>
    /// Strip leaked bold/italic tags from translated text.
    /// Our translations in the DB never contain <b>/<i> tags — if they appear
    /// in the final output, they leaked from the English source during
    /// segment-based or partial text replacement.
    /// </summary>
    private static string CleanOrphanBoldTags(string text)
    {
        if (string.IsNullOrEmpty(text))
            return text;

        // v2: translations include intentional <b> tags from ko_formatted.
        // Only strip truly orphan tags (unmatched open/close), not all bold.
        int opens = 0, closes = 0;
        int idx = 0;
        while ((idx = text.IndexOf("<b>", idx, StringComparison.Ordinal)) >= 0) { opens++; idx += 3; }
        idx = 0;
        while ((idx = text.IndexOf("</b>", idx, StringComparison.Ordinal)) >= 0) { closes++; idx += 4; }

        if (opens == closes)
            return text; // balanced — keep all tags

        // Unbalanced: strip all (safety fallback)
        if (opens > 0 || closes > 0)
            text = text.Replace("<b>", "").Replace("</b>", "");

        return text;
    }

    private static bool TryTranslateRuntimeLexicon(ref string value)
    {
        if (string.IsNullOrWhiteSpace(value))
        {
            return false;
        }

        var originalValue = value;
        var updated = value;
        var changed = false;

        foreach (var replacement in RuntimeSubstringReplacements)
        {
            if (replacement.Key.Length == 0 || !updated.Contains(replacement.Key, StringComparison.Ordinal))
            {
                continue;
            }

            updated = updated.Replace(replacement.Key, replacement.Value, StringComparison.Ordinal);
            changed = true;
        }

        foreach (var rule in RuntimeRegexRules)
        {
            var rewritten = rule.Regex.Replace(updated, rule.Replace);
            if (!string.Equals(rewritten, updated, StringComparison.Ordinal))
            {
                updated = rewritten;
                changed = true;
            }
        }

        if (!changed || string.Equals(updated, originalValue, StringComparison.Ordinal))
        {
            return false;
        }

        value = updated;
        lock (StateLock)
        {
            _runtimeLexiconHitCount++;
        }
        RecordTranslationHit(originalValue, updated);
        return true;
    }

    private static bool TryTranslateGeneratedPattern(ref string value)
    {
        var originalValue = value;

        var abilityMatch = Regex.Match(value, @"^Ability Scores - Remaining Points: (?<points>\d+)$", RegexOptions.CultureInvariant);
        if (abilityMatch.Success)
        {
            value = $"능력치 - 남은 포인트: {abilityMatch.Groups["points"].Value}";
            RecordTranslationHit(originalValue, value);
            return true;
        }

        var bonusMatch = Regex.Match(value, @"^(?<bonus>\+\d+)\s+(?<stat>Strength|Dexterity|Constitution|Intelligence|Wisdom|Charisma)$", RegexOptions.CultureInvariant);
        if (bonusMatch.Success &&
            StatNameOverrides.TryGetValue(bonusMatch.Groups["stat"].Value, out var statKo))
        {
            value = $"{bonusMatch.Groups["bonus"].Value} {statKo}";
            RecordTranslationHit(originalValue, value);
            return true;
        }

        var hpMatch = Regex.Match(value, @"^(?<current>\d+)\/(?<max>\d+)\s+HP$", RegexOptions.CultureInvariant);
        if (hpMatch.Success)
        {
            value = $"{hpMatch.Groups["current"].Value}/{hpMatch.Groups["max"].Value} 체력";
            RecordTranslationHit(originalValue, value);
            return true;
        }

        var levelMatch = Regex.Match(value, @"^(?<level>\d+)(?:st|nd|rd|th)\s+Level$", RegexOptions.CultureInvariant);
        if (levelMatch.Success)
        {
            value = $"{levelMatch.Groups["level"].Value}레벨";
            RecordTranslationHit(originalValue, value);
            return true;
        }

        var spellslotMatch = Regex.Match(value, @"^(?<level>\d+)(?:st|nd|rd|th)\s+Level\s+Spellslot\s+(?<delta>[+\-]\d+)$", RegexOptions.CultureInvariant);
        if (spellslotMatch.Success)
        {
            value = $"{spellslotMatch.Groups["level"].Value}레벨 주문 슬롯 {spellslotMatch.Groups["delta"].Value}";
            RecordTranslationHit(originalValue, value);
            return true;
        }

        var dcResultMatch = Regex.Match(
            value,
            @"^(?<delta>[+\-]\d+)\s+DC(?<tail>(?:</color>|<[^>]+>|\s)*)\|\s*(?<body>.+)$",
            RegexOptions.CultureInvariant);
        if (dcResultMatch.Success)
        {
            var translatedBody = TranslateShortSystemPhrase(dcResultMatch.Groups["body"].Value);
            if (!string.Equals(translatedBody, dcResultMatch.Groups["body"].Value, StringComparison.Ordinal))
            {
                value = $"{dcResultMatch.Groups["delta"].Value} DC{dcResultMatch.Groups["tail"].Value}| {translatedBody}";
                RecordTranslationHit(originalValue, value);
                return true;
            }
        }

        if (value.Contains("PR, '", StringComparison.Ordinal))
        {
            value = value.Replace("PR, '", "부활 가능성(PR), '", StringComparison.Ordinal);
            RecordTranslationHit(originalValue, value);
            return true;
        }

        if (value.Contains("RF가 없었다.", StringComparison.Ordinal))
        {
            value = value.Replace("RF가 없었다.", "부활 기금(RF)이 없었다.", StringComparison.Ordinal);
            RecordTranslationHit(originalValue, value);
            return true;
        }

        var translatedFragments = TranslateRichSystemFragments(value);
        if (!string.Equals(translatedFragments, value, StringComparison.Ordinal))
        {
            value = translatedFragments;
            RecordTranslationHit(originalValue, value);
            return true;
        }

        return false;
    }

    private static string TranslateShortSystemPhrase(string value)
    {
        if (string.IsNullOrWhiteSpace(value))
        {
            return value;
        }

        var updated = value;
        updated = updated.Replace("You managed to shove him!", "그를 밀쳐내는 데 성공했다!", StringComparison.Ordinal);
        updated = updated.Replace("You've pissed him off.", "그를 화나게 만들었다.", StringComparison.Ordinal);
        updated = updated.Replace("It's started to drain you.", "당신을 서서히 약화시키기 시작했다.", StringComparison.Ordinal);
        updated = updated.Replace("A lot of bad apples removed.", "상한 사과를 많이 치웠다.", StringComparison.Ordinal);
        updated = updated.Replace("You ate most of them, honestly.", "솔직히 말해 대부분을 먹어버렸다.", StringComparison.Ordinal);
        updated = updated.Replace("You ate some extra.", "조금 더 먹어버렸다.", StringComparison.Ordinal);
        updated = updated.Replace("There's a zombie downstairs.", "아래층에 좀비가 있다.", StringComparison.Ordinal);
        updated = updated.Replace("The zombie moved first.", "좀비가 먼저 움직였다.", StringComparison.Ordinal);
        return updated;
    }

    private static string TranslateRichSystemFragments(string value)
    {
        if (string.IsNullOrWhiteSpace(value))
        {
            return value;
        }

        var updated = value;
        foreach (var pair in StatNameOverrides)
        {
            updated = Regex.Replace(
                updated,
                $@"\b{Regex.Escape(pair.Key)}\b",
                pair.Value,
                RegexOptions.CultureInvariant);
        }

        updated = Regex.Replace(updated, @"\bCollected Spells\b", "수집한 주문", RegexOptions.CultureInvariant);
        updated = Regex.Replace(updated, @"\bAll Items\b", "모든 아이템", RegexOptions.CultureInvariant);
        updated = Regex.Replace(updated, @"\bCantrips\b", "캔트립", RegexOptions.CultureInvariant);
        updated = Regex.Replace(updated, @"\bSpell Slots\b", "주문 슬롯", RegexOptions.CultureInvariant);
        updated = Regex.Replace(updated, @"\bSpellslot\b", "주문 슬롯", RegexOptions.CultureInvariant);
        updated = Regex.Replace(updated, @"\bHit Points\b", "체력", RegexOptions.CultureInvariant);
        updated = Regex.Replace(updated, @"\bHP\b", "체력", RegexOptions.CultureInvariant);

        return updated;
    }

    private void LoadEntriesFromJson(string json)
    {
        using var doc = JsonDocument.Parse(json, new JsonDocumentOptions
        {
            CommentHandling = JsonCommentHandling.Skip,
            AllowTrailingCommas = true
        });

        if (doc.RootElement.ValueKind == JsonValueKind.Array)
        {
            foreach (var item in doc.RootElement.EnumerateArray())
            {
                AddEntry(item);
            }
            return;
        }

        if (doc.RootElement.ValueKind == JsonValueKind.Object &&
            doc.RootElement.TryGetProperty("entries", out var entries) &&
            entries.ValueKind == JsonValueKind.Array)
        {
            foreach (var item in entries.EnumerateArray())
            {
                AddEntry(item);
            }

            if (doc.RootElement.TryGetProperty("contextual_entries", out var contextualEntries) &&
                contextualEntries.ValueKind == JsonValueKind.Array)
            {
                foreach (var item in contextualEntries.EnumerateArray())
                {
                    AddContextualEntry(item);
                }
            }
            return;
        }

        Log.LogWarning("Unsupported translations.json shape. Expected array or object with entries[].");
    }

    private static void AddEntry(JsonElement item)
    {
        var source = ReadString(item, "source_text") ?? ReadString(item, "source");
        var target = ReadString(item, "target_text") ?? ReadString(item, "target");
        var status = ReadString(item, "status");

        if (string.IsNullOrWhiteSpace(source) || string.IsNullOrWhiteSpace(target))
        {
            return;
        }

        if (!string.IsNullOrWhiteSpace(status) &&
            !string.Equals(status, "translated", StringComparison.OrdinalIgnoreCase) &&
            !string.Equals(status, "reviewed", StringComparison.OrdinalIgnoreCase))
        {
            return;
        }

        TranslationMap[source] = target;
        var normalized = NormalizeKey(source);
        if (normalized.Length > 0 && !NormalizedMap.ContainsKey(normalized))
        {
            NormalizedMap[normalized] = target;
        }
    }


    private void LoadTextAssetOverrides()
    {
        var candidateDirs = new[]
        {
            Path.Combine(Paths.GameRootPath, "Esoteric Ebb_Data", "StreamingAssets", "TranslationPatch", "localizationtexts"),
            Path.Combine(Paths.GameRootPath, "StreamingAssets", "TranslationPatch", "localizationtexts"),
            Path.Combine(Paths.GameRootPath, "Esoteric Ebb_Data", "StreamingAssets", "TranslationPatch", "textassets"),
            Path.Combine(Paths.GameRootPath, "StreamingAssets", "TranslationPatch", "textassets")
        };

        var dirs = candidateDirs.Where(Directory.Exists).Distinct(StringComparer.OrdinalIgnoreCase).ToArray();
        if (dirs.Length == 0)
        {
            return;
        }

        try
        {
            var patterns = new[] { "*.txt", "*.json" };
            foreach (var dir in dirs)
            {
                foreach (var pattern in patterns)
                {
                    foreach (var path in Directory.EnumerateFiles(dir, pattern, SearchOption.AllDirectories))
                    {
                        var name = Path.GetFileNameWithoutExtension(path);
                        var text = File.ReadAllText(path);
                        if (string.IsNullOrWhiteSpace(name) || string.IsNullOrWhiteSpace(text))
                        {
                            continue;
                        }
                        TextAssetOverrides[name] = text;
                    }
                }
            }
            _textAssetOverrideCount = TextAssetOverrides.Count;
            Log.LogInfo($"Loaded text asset overrides from {dirs.Length} directories ({_textAssetOverrideCount} files)");
        }
        catch (Exception ex)
        {
            Log.LogWarning($"Failed to load text asset overrides: {ex.Message}");
        }
    }

    private void LoadLocalizationIdOverrides()
    {
        var candidates = new[]
        {
            Path.Combine(Paths.GameRootPath, "Esoteric Ebb_Data", "StreamingAssets", "TranslationPatch", "localizationtexts"),
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
                    if (string.IsNullOrWhiteSpace(id) || string.IsNullOrWhiteSpace(ko))
                    {
                        continue;
                    }

                    LocalizationIdOverrides[id] = ko;
                }
            }

            _localizationIdOverrideCount = LocalizationIdOverrides.Count;
            Log.LogInfo($"Loaded localization ID overrides: {dir} ({_localizationIdOverrideCount} ids)");
        }
        catch (Exception ex)
        {
            Log.LogWarning($"Failed to load localization ID overrides: {ex.Message}");
        }
    }

    private void LoadRuntimeLexicon()
    {
        var candidates = new[]
        {
            Path.Combine(Paths.GameRootPath, "Esoteric Ebb_Data", "StreamingAssets", "TranslationPatch", "runtime_lexicon.json"),
            Path.Combine(Paths.GameRootPath, "StreamingAssets", "TranslationPatch", "runtime_lexicon.json")
        };

        var path = candidates.FirstOrDefault(File.Exists);
        if (path is null)
        {
            return;
        }

        try
        {
            RuntimeSubstringReplacements.Clear();
            RuntimeRegexRules.Clear();

            using var doc = JsonDocument.Parse(File.ReadAllText(path));
            if (doc.RootElement.ValueKind != JsonValueKind.Object)
            {
                return;
            }

            if (doc.RootElement.TryGetProperty("substring_replacements", out var substringReplacements) &&
                substringReplacements.ValueKind == JsonValueKind.Array)
            {
                foreach (var item in substringReplacements.EnumerateArray())
                {
                    var find = ReadString(item, "find");
                    var replace = ReadString(item, "replace");
                    if (string.IsNullOrEmpty(find) || replace is null)
                    {
                        continue;
                    }

                    RuntimeSubstringReplacements.Add(new KeyValuePair<string, string>(find, replace));
                }
            }

            RuntimeSubstringReplacements.Sort((a, b) => b.Key.Length.CompareTo(a.Key.Length));

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

                    var ignoreCase = item.TryGetProperty("ignore_case", out var ignoreCaseEl) && ignoreCaseEl.ValueKind == JsonValueKind.True;
                    var options = RegexOptions.CultureInvariant;
                    if (ignoreCase)
                    {
                        options |= RegexOptions.IgnoreCase;
                    }

                    RuntimeRegexRules.Add(new RuntimeRegexRule
                    {
                        Name = ReadString(item, "name") ?? string.Empty,
                        Regex = new Regex(pattern, options),
                        Replace = replace
                    });
                }
            }

            _runtimeLexiconSubstringCount = RuntimeSubstringReplacements.Count;
            _runtimeLexiconRegexCount = RuntimeRegexRules.Count;
            Log.LogInfo($"Loaded runtime lexicon: {path} ({_runtimeLexiconSubstringCount} substrings, {_runtimeLexiconRegexCount} regex rules)");
        }
        catch (Exception ex)
        {
            Log.LogWarning($"Failed to load runtime lexicon: {ex.Message}");
        }
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

    private static void AddContextualEntry(JsonElement item)
    {
        var source = ReadString(item, "source");
        var target = ReadString(item, "target");
        var status = ReadString(item, "status");
        if (string.IsNullOrWhiteSpace(source) || string.IsNullOrWhiteSpace(target))
        {
            return;
        }

        if (!string.IsNullOrWhiteSpace(status) &&
            !string.Equals(status, "translated", StringComparison.OrdinalIgnoreCase) &&
            !string.Equals(status, "reviewed", StringComparison.OrdinalIgnoreCase))
        {
            return;
        }

        var normalized = NormalizeKey(source);
        if (normalized.Length == 0)
        {
            return;
        }

        if (!ContextualMap.TryGetValue(normalized, out var entries))
        {
            entries = new List<ContextualEntry>();
            ContextualMap[normalized] = entries;
        }

        entries.Add(new ContextualEntry
        {
            Id = ReadString(item, "id") ?? string.Empty,
            Source = source,
            Target = target,
            ContextEn = NormalizeKey(ReadString(item, "context_en") ?? string.Empty),
            SpeakerHint = ReadString(item, "speaker_hint") ?? string.Empty,
            TextRole = ReadString(item, "text_role") ?? string.Empty,
            TranslationLane = ReadString(item, "translation_lane") ?? string.Empty,
            SourceFile = ReadString(item, "source_file") ?? string.Empty
        });
        _contextualLoadedCount++;
    }

    private static string? ReadString(JsonElement item, string property)
    {
        if (!item.TryGetProperty(property, out var value) || value.ValueKind != JsonValueKind.String)
        {
            return null;
        }

        return value.GetString();
    }

    private bool ApplyTextPatch(Harmony harmony, string[] assemblyCandidates, string typeName, string propertyName, string prefixMethodName)
    {
        try
        {
            var type = FindTypeInAssemblies(assemblyCandidates, typeName);
            if (type is null)
            {
                Log.LogWarning($"Patch deferred: type not found ({typeName}) in [{string.Join(", ", assemblyCandidates)}]");
                return false;
            }

            var setter = type.GetProperty(propertyName, BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic)?.SetMethod;
            if (setter is null)
            {
                Log.LogWarning($"Patch skipped: setter not found ({typeName}.{propertyName}).");
                return false;
            }

            var prefix = typeof(Plugin).GetMethod(prefixMethodName, BindingFlags.Static | BindingFlags.NonPublic);
            if (prefix is null)
            {
                Log.LogWarning($"Patch skipped: prefix method not found ({prefixMethodName}).");
                return false;
            }

            harmony.Patch(setter, prefix: new HarmonyMethod(prefix));
            Log.LogInfo($"Patch applied: {typeName}.{propertyName} setter");
            return true;
        }
        catch (Exception ex)
        {
            Log.LogError($"Patch failed: {typeName}.{propertyName} => {ex.GetType().Name}: {ex.Message}");
            return false;
        }
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

    private static void TmpTextPrefix(ref string value)
    {
        if (!EnterPatchedCall())
        {
            return;
        }

        try
        {
            TryTranslate(ref value, "tmp_text");
        }
        finally
        {
            ExitPatchedCall();
        }
    }

    private bool ApplyTextAssetPatch(Harmony harmony)
    {
        try
        {
            var type = FindTypeInAssemblies(new[] { "UnityEngine.TextRenderingModule", "UnityEngine.CoreModule", "UnityEngine" }, "UnityEngine.TextAsset");
            if (type is null)
            {
                Log.LogWarning("Patch deferred: type not found (UnityEngine.TextAsset).");
                return false;
            }

            var getter = type.GetProperty("text", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic)?.GetMethod;
            if (getter is null)
            {
                Log.LogWarning("Patch skipped: getter not found (UnityEngine.TextAsset.text).");
                return false;
            }

            var postfix = typeof(Plugin).GetMethod(nameof(TextAssetTextPostfix), BindingFlags.Static | BindingFlags.NonPublic);
            if (postfix is null)
            {
                Log.LogWarning("Patch skipped: postfix method not found (TextAssetTextPostfix).");
                return false;
            }

            harmony.Patch(getter, postfix: new HarmonyMethod(postfix));
            Log.LogInfo("Patch applied: UnityEngine.TextAsset.text getter");
            return true;
        }
        catch (Exception ex)
        {
            Log.LogError($"Patch failed: UnityEngine.TextAsset.text => {ex.GetType().Name}: {ex.Message}");
            return false;
        }
    }

    private bool ApplyMethodPatch(Harmony harmony, string[] assemblyCandidates, string typeName, string methodName, string patchMethodName, int parameterCount, bool usePostfix = false)
    {
        try
        {
            var type = FindTypeInAssemblies(assemblyCandidates, typeName);
            if (type is null)
            {
                Log.LogWarning($"Patch deferred: type not found ({typeName}) in [{string.Join(", ", assemblyCandidates)}]");
                return false;
            }

            var method = type.GetMethods(BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic)
                .FirstOrDefault(m => string.Equals(m.Name, methodName, StringComparison.Ordinal) &&
                                     m.GetParameters().Length == parameterCount);
            if (method is null)
            {
                Log.LogWarning($"Patch skipped: method not found ({typeName}.{methodName}/{parameterCount}).");
                return false;
            }

            var patchMethod = typeof(Plugin).GetMethod(patchMethodName, BindingFlags.Static | BindingFlags.NonPublic);
            if (patchMethod is null)
            {
                Log.LogWarning($"Patch skipped: patch method not found ({patchMethodName}).");
                return false;
            }

            if (usePostfix)
            {
                harmony.Patch(method, postfix: new HarmonyMethod(patchMethod));
            }
            else
            {
                harmony.Patch(method, prefix: new HarmonyMethod(patchMethod));
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

    private static void UiTextPrefix(ref string value)
    {
        if (!EnterPatchedCall())
        {
            return;
        }

        try
        {
            TryTranslate(ref value, "ui_text");
        }
        finally
        {
            ExitPatchedCall();
        }
    }

    private static void UiElementsTextPrefix(object? __instance, ref string value)
    {
        if (!EnterPatchedCall())
        {
            return;
        }

        try
        {
            CaptureUiToolkitText(__instance, value);
            TryTranslate(ref value, "ui_elements");
        }
        finally
        {
            ExitPatchedCall();
        }
    }

    private static void DialogStartDialogPrefix(object? inkAsset)
    {
        _currentDialogSourceFile = ExtractUnityObjectName(inkAsset);
    }

    private static void DialogAddTextPrefix(ref string text)
    {
        TryTranslate(ref text, "ink_dialogue");
    }

    private static void DialogAddChoiceTextPrefix(object? __instance, ref string text)
    {
        TryLogAddChoiceSignature(__instance);

        if (string.IsNullOrWhiteSpace(text))
        {
            return;
        }

        CaptureChoice(text);

        // AddChoiceText receives plain body text (no link/numbering wrapper).
        // Game adds <link="N">N.   ...</link> AFTER this hook via DialogManager.
        // Just translate the body — game handles numbering independently.
        TryTranslate(ref text, "ink_choice");
    }

    private static void TryLogAddChoiceSignature(object? instance)
    {
        if (instance is null || _choiceSignatureLogged)
        {
            return;
        }

        try
        {
            var method = instance.GetType()
                .GetMethods(BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic)
                .FirstOrDefault(m => string.Equals(m.Name, "AddChoiceText", StringComparison.Ordinal) &&
                                     m.GetParameters().Length == 9);
            if (method is null)
            {
                return;
            }

            var paramNames = string.Join(", ", method.GetParameters()
                .Select(p => $"{p.ParameterType.Name} {p.Name}"));
            _choiceSignatureLogged = true;
            LogSource?.LogInfo($"[DIAG] AddChoiceText params: {paramNames}");
        }
        catch (Exception ex)
        {
            LogSource?.LogWarning($"[DIAG] Failed to inspect AddChoiceText params: {ex.Message}");
        }
    }

    private static void TmpOnEnablePostfix(object? __instance)
    {
        TryInjectFallbackFont(__instance);
        TranslateCurrentTextProperty(__instance);
    }

    private static void UiOnEnablePostfix(object? __instance)
    {
        TranslateCurrentTextProperty(__instance);
    }

    private static void TmpConcreteAwakePostfix(object? __instance)
    {
        TryInjectFallbackFont(__instance);
        TranslateCurrentTextProperty(__instance);
        TriggerSceneTextScan("tmp_concrete_awake");
    }

    private static void LocalizeCheckLanguagePostfix(string ID, ref string __result)
    {
        if (string.IsNullOrWhiteSpace(ID) || LocalizationIdOverrides.Count == 0)
        {
            return;
        }

        if (LocalizationIdOverrides.TryGetValue(ID, out var replacement) && !string.IsNullOrWhiteSpace(replacement))
        {
            __result = replacement;
            lock (StateLock)
            {
                _localizationIdOverrideHitCount++;
            }
            return;
        }

        lock (StateLock)
        {
            if (_localizationIdOverrideMissLoggedCount < 80 && LocalizationMissSeen.Add(ID))
            {
                _localizationIdOverrideMissLoggedCount++;
                LogSource?.LogInfo($"Localization ID miss: {ID}");
            }
        }
    }

    private static void TryApplyMenuDirectOverride(object component)
    {
        if (!EnterPatchedCall())
        {
            return;
        }

        try
        {
            var prop = component.GetType().GetProperty("text", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
            if (prop is null || !prop.CanRead || !prop.CanWrite)
            {
                return;
            }

            if (prop.GetValue(component) is not string currentText || string.IsNullOrWhiteSpace(currentText))
            {
                return;
            }

            var trimmed = currentText.Trim();
            if (!MenuDirectOverrides.TryGetValue(trimmed, out var replacement))
            {
                return;
            }

            if (string.Equals(currentText, replacement, StringComparison.Ordinal))
            {
                return;
            }

            prop.SetValue(component, replacement);
            lock (StateLock)
            {
                _menuDirectOverrideHits++;
            }
            RecordTranslationHit(currentText, replacement);
        }
        catch
        {
        }
        finally
        {
            ExitPatchedCall();
        }
    }

    private static void MenuControllerStartPostfix(object? __instance)
    {
        TranslateMenuControllerUI(__instance);
    }

    private static void MenuControllerRefreshPostfix(object? __instance)
    {
        TranslateMenuControllerUI(__instance);
    }

    private static void MenuControllerUpdatePostfix(object? __instance)
    {
        if (__instance is null)
        {
            return;
        }

        try
        {
            var countField = __instance.GetType().GetField("SkipMessageCounter", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
            if (countField is null)
            {
                return;
            }

            if (countField.GetValue(__instance) is not int tick)
            {
                return;
            }

            if (_menuSceneSweepCount < 20 && tick % 10 == 0)
            {
                TranslateMenuControllerUI(__instance);
            }
        }
        catch
        {
        }
    }

    private static void TranslateMenuControllerUI(object? menuController)
    {
        TranslateActiveSceneUI(applyTranslations: true);
    }

    private static void SceneLoadedPostfix(object? scene, object? mode)
    {
        TriggerSceneTextScan("scene_loaded");
        if (_fullCaptureEnabled)
        {
            lock (StateLock)
            {
                FlushFullCaptureUnsafe();
            }
        }
    }

    private static void TriggerSceneTextScan(string reason)
    {
        var sceneName = GetActiveSceneName();
        if (string.IsNullOrWhiteSpace(sceneName))
        {
            sceneName = "<unknown>";
        }

        var key = $"{sceneName}|{reason}";
        lock (StateLock)
        {
            if (!TriggeredSceneScans.Add(key))
            {
                return;
            }
            _sceneTextScanCount++;
        }

        LogSource?.LogInfo($"Scene text scan trigger: {sceneName} ({reason})");
        TranslateActiveSceneUI(applyTranslations: true);
    }

    private static void TranslateActiveSceneUI(bool applyTranslations)
    {
        try
        {
            var resourcesType = FindTypeByName("UnityEngine.Resources");
            var tmpTextType = FindTypeByName("TMPro.TMP_Text");
            var uiTextType = FindTypeByName("UnityEngine.UI.Text");
            if (resourcesType is null || tmpTextType is null)
            {
                return;
            }

            var findAll = resourcesType.GetMethods(BindingFlags.Public | BindingFlags.Static)
                .FirstOrDefault(m =>
                {
                    if (!string.Equals(m.Name, "FindObjectsOfTypeAll", StringComparison.Ordinal))
                    {
                        return false;
                    }
                    var parameters = m.GetParameters();
                    return parameters.Length == 1 && parameters[0].ParameterType == typeof(Type);
                });
            if (findAll is null)
            {
                return;
            }

            if (findAll.Invoke(null, new object[] { tmpTextType }) is not System.Collections.IEnumerable objects)
            {
                return;
            }

            var translatedBefore = _translateHitCount;
            var objectCount = 0;
            foreach (var tmp in objects)
            {
                objectCount++;
                if (applyTranslations)
                {
                    TranslateCurrentTextProperty(tmp);
                    TryApplyMenuDirectOverride(tmp);
                }
            }

            WriteMenuRuntimeDump(findAll, tmpTextType, uiTextType);

            lock (StateLock)
            {
                _menuSceneSweepCount++;
                if (applyTranslations)
                {
                    _menuSceneTranslatedCount += Math.Max(0, _translateHitCount - translatedBefore);
                }
            }

            LogSource?.LogInfo($"Scene text sweep complete: objects={objectCount}, applyTranslations={applyTranslations}, translated_delta={Math.Max(0, _translateHitCount - translatedBefore)}, direct_hits={_menuDirectOverrideHits}");
        }
        catch (Exception ex)
        {
            LogSource?.LogWarning($"Scene text sweep failed: {ex.Message}");
        }
    }

    private static void WriteMenuRuntimeDump(MethodInfo findAll, Type tmpTextType, Type? uiTextType)
    {
        if (string.IsNullOrWhiteSpace(_menuDumpPath))
        {
            return;
        }

        var sceneName = GetActiveSceneName();
        if (string.IsNullOrWhiteSpace(sceneName))
        {
            sceneName = "<unknown>";
        }

        lock (StateLock)
        {
            if (!DumpedMenuScenes.Add(sceneName))
            {
                return;
            }
        }

        try
        {
            var entries = new List<object>();
            CollectMenuTextEntries(findAll, tmpTextType, "TMP_Text", entries);
            if (uiTextType is not null)
            {
                CollectMenuTextEntries(findAll, uiTextType, "UI.Text", entries);
            }

            var payload = new
            {
                written_at = DateTime.Now.ToString("s"),
                plugin_version = PluginVersion,
                scene = sceneName,
                entry_count = entries.Count,
                entries
            };

            RunWithSuppressedDiagnostics(() =>
                File.WriteAllText(_menuDumpPath, JsonSerializer.Serialize(payload, new JsonSerializerOptions
                {
                    WriteIndented = true
                })));

            LogSource?.LogInfo($"Wrote menu runtime dump: {_menuDumpPath} ({entries.Count} entries)");
        }
        catch (Exception ex)
        {
            LogSource?.LogWarning($"Failed to write menu runtime dump: {ex.Message}");
        }
    }

    private static void CollectMenuTextEntries(MethodInfo findAll, Type targetType, string kind, List<object> entries)
    {
        if (findAll.Invoke(null, new object[] { targetType }) is not System.Collections.IEnumerable objects)
        {
            return;
        }

        foreach (var obj in objects)
        {
            if (obj is null)
            {
                continue;
            }

            entries.Add(new
            {
                kind,
                object_name = ExtractUnityObjectName(obj),
                path = GetTransformPath(obj),
                text = ReadStringProperty(obj, "text"),
                component_type = obj.GetType().FullName ?? obj.GetType().Name,
                attached_components = GetAttachedComponentTypes(obj)
            });
        }
    }

    private static string ReadStringProperty(object instance, string propertyName)
    {
        try
        {
            var prop = instance.GetType().GetProperty(propertyName, BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
            if (prop?.GetValue(instance) is string value)
            {
                return value;
            }
        }
        catch
        {
        }

        return string.Empty;
    }

    private static string[] GetAttachedComponentTypes(object instance)
    {
        try
        {
            var gameObjectProp = instance.GetType().GetProperty("gameObject", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
            var gameObject = gameObjectProp?.GetValue(instance);
            if (gameObject is null)
            {
                return Array.Empty<string>();
            }

            var componentBaseType = FindTypeByName("UnityEngine.Component");
            var getComponents = gameObject.GetType().GetMethods(BindingFlags.Instance | BindingFlags.Public)
                .FirstOrDefault(m => string.Equals(m.Name, "GetComponents", StringComparison.Ordinal) && m.GetParameters().Length == 1);
            if (componentBaseType is null || getComponents is null)
            {
                return Array.Empty<string>();
            }

            if (getComponents.Invoke(gameObject, new object[] { componentBaseType }) is not System.Collections.IEnumerable components)
            {
                return Array.Empty<string>();
            }

            var names = new List<string>();
            foreach (var component in components)
            {
                if (component is null)
                {
                    continue;
                }

                names.Add(component.GetType().FullName ?? component.GetType().Name);
            }

            return names.ToArray();
        }
        catch
        {
            return Array.Empty<string>();
        }
    }

    private static string GetTransformPath(object instance)
    {
        try
        {
            var transformProp = instance.GetType().GetProperty("transform", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
            var current = transformProp?.GetValue(instance);
            if (current is null)
            {
                return string.Empty;
            }

            var names = new List<string>();
            while (current is not null)
            {
                var name = ExtractUnityObjectName(current);
                if (!string.IsNullOrWhiteSpace(name))
                {
                    names.Add(name);
                }

                var parentProp = current.GetType().GetProperty("parent", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
                current = parentProp?.GetValue(current);
            }

            names.Reverse();
            return string.Join("/", names);
        }
        catch
        {
            return string.Empty;
        }
    }

    private static string GetActiveSceneName()
    {
        try
        {
            var sceneManagerType = FindTypeByName("UnityEngine.SceneManagement.SceneManager");
            var getActiveScene = sceneManagerType?.GetMethod("GetActiveScene", BindingFlags.Public | BindingFlags.Static);
            var scene = getActiveScene?.Invoke(null, Array.Empty<object>());
            if (scene is null)
            {
                return string.Empty;
            }

            var nameProp = scene.GetType().GetProperty("name", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
            return nameProp?.GetValue(scene) as string ?? string.Empty;
        }
        catch
        {
            return string.Empty;
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

        if (!TextAssetOverrides.TryGetValue(assetName, out var replacement) || string.IsNullOrEmpty(replacement))
        {
            lock (StateLock)
            {
                if (_textAssetOverrideMissLoggedCount < 40 && TextAssetOverrideMissSeen.Add(assetName))
                {
                    _textAssetOverrideMissLoggedCount++;
                    LogSource?.LogInfo($"TextAsset override miss: {assetName}");
                }
            }
            return;
        }

        __result = replacement;
        lock (StateLock)
        {
            _textAssetOverrideHitCount++;
        }
    }

    private static void TranslateCurrentTextProperty(object? instance)
    {
        if (instance is null || !EnterPatchedCall())
        {
            return;
        }

        try
        {
            var prop = instance.GetType().GetProperty("text", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
            if (prop is null || !prop.CanRead || !prop.CanWrite)
            {
                return;
            }

            if (prop.GetValue(instance) is not string currentText || string.IsNullOrWhiteSpace(currentText))
            {
                return;
            }

            lock (StateLock)
            {
                if (_menuSweepSampleCount < 60)
                {
                    _menuSweepSampleCount++;
                    LogSource?.LogInfo($"Menu text sample: {currentText}");
                }
            }

            var translated = currentText;
            if (!TryTranslate(ref translated, "menu_scan") || string.Equals(translated, currentText, StringComparison.Ordinal))
            {
                return;
            }

            prop.SetValue(instance, translated);
        }
        catch
        {
        }
        finally
        {
            ExitPatchedCall();
        }
    }


    private static bool TryTranslateContextual(ref string value, string originalValue, string normalized)
    {
        if (!ContextualMap.TryGetValue(normalized, out var candidates) || candidates.Count == 0)
        {
            return false;
        }

        var currentSourceFile = NormalizeSourceFile(_currentDialogSourceFile);
        if (!string.IsNullOrEmpty(currentSourceFile))
        {
            var sourceFileCandidates = candidates
                .Where(candidate => string.Equals(NormalizeSourceFile(candidate.SourceFile), currentSourceFile, StringComparison.Ordinal))
                .ToArray();
            if (sourceFileCandidates.Length == 1)
            {
                value = sourceFileCandidates[0].Target;
                lock (StateLock)
                {
                    _contextualHitCount++;
                    _sourceFileContextHitCount++;
                }
                RecordTranslationHit(originalValue, value);
                return true;
            }
        }

        if (!ShouldUseContextualLookup(originalValue, normalized))
        {
            return false;
        }

        var history = SnapshotRecentHistory();
        ContextualEntry? best = null;
        var bestScore = 0;
        var tied = false;

        foreach (var candidate in candidates)
        {
            var score = ScoreContextualCandidate(candidate, history);
            if (score <= 0)
            {
                continue;
            }

            if (score > bestScore)
            {
                best = candidate;
                bestScore = score;
                tied = false;
                continue;
            }

            if (score == bestScore && best is not null &&
                !string.Equals(best.Target, candidate.Target, StringComparison.Ordinal))
            {
                tied = true;
            }
        }

        if (best is null || tied)
        {
            return false;
        }

        value = RestoreChoicePrefix(originalValue, best.Target);
        lock (StateLock)
        {
            _contextualHitCount++;
        }
        RecordTranslationHit(originalValue, value);
        return true;
    }


    private static string RestoreChoicePrefix(string original, string translated)
    {
        if (string.IsNullOrWhiteSpace(original) || string.IsNullOrWhiteSpace(translated))
        {
            return translated;
        }

        var match = Regex.Match(original, @"^\s*(\d+)(\.\s+|\s+)");
        if (!match.Success)
        {
            return translated;
        }

        var translatedTrimmed = translated.TrimStart();
        if (Regex.IsMatch(translatedTrimmed, @"^\d+(\.\s+|\s+)"))
        {
            return translated;
        }

        return match.Value + translatedTrimmed;
    }


    private static string NormalizeKey(string input)
    {
        if (string.IsNullOrWhiteSpace(input))
        {
            return string.Empty;
        }

        var text = input.Trim();
        // Strip ALL tags (opening, closing, self-closing, color codes) not just leading/trailing
        text = Regex.Replace(text, @"<[^>]+>", "");
        text = Regex.Replace(text, @"^\d+(?:\.\s+|\s+)", "");
        text = StripGameplayPrefix(text);
        text = StripOuterQuotes(text);
        return Regex.Replace(text, @"\s+", " ").Trim();
    }

    private static bool ShouldUseContextualLookup(string originalValue, string normalized)
    {
        if (normalized.Length == 0)
        {
            return false;
        }

        if (normalized.Length > 32)
        {
            return false;
        }

        var wordCount = normalized.Split(' ', StringSplitOptions.RemoveEmptyEntries).Length;
        if (wordCount > 4)
        {
            return false;
        }

        if (!Regex.IsMatch(normalized, "[A-Za-z]"))
        {
            return false;
        }

        return true;
    }

    private static int ScoreContextualCandidate(ContextualEntry candidate, IReadOnlyList<string> history)
    {
        var score = 0;
        var currentSourceFile = NormalizeSourceFile(_currentDialogSourceFile);
        if (!string.IsNullOrEmpty(currentSourceFile) &&
            string.Equals(NormalizeSourceFile(candidate.SourceFile), currentSourceFile, StringComparison.Ordinal))
        {
            score += 8;
        }

        if (!string.IsNullOrEmpty(candidate.ContextEn))
        {
            foreach (var previous in history)
            {
                if (previous.Length < 4)
                {
                    continue;
                }

                if (!candidate.ContextEn.Contains(previous, StringComparison.Ordinal))
                {
                    continue;
                }

                score += previous.Length >= 12 ? 3 : 2;
            }
        }

        if (!string.IsNullOrEmpty(candidate.SpeakerHint))
        {
            score += 1;
        }

        if (string.Equals(candidate.TextRole, "dialogue", StringComparison.OrdinalIgnoreCase) ||
            string.Equals(candidate.TextRole, "choice", StringComparison.OrdinalIgnoreCase))
        {
            score += 1;
        }

        return score;
    }

    private static string NormalizeSourceFile(string? sourceFile)
    {
        return string.IsNullOrWhiteSpace(sourceFile)
            ? string.Empty
            : sourceFile.Trim();
    }

    private static string ExtractUnityObjectName(object? value)
    {
        if (value is null)
        {
            return string.Empty;
        }

        try
        {
            var property = value.GetType().GetProperty("name", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
            if (property?.GetValue(value) is string name && !string.IsNullOrWhiteSpace(name))
            {
                return name.Trim();
            }
        }
        catch
        {
        }

        return string.Empty;
    }

    private static IReadOnlyList<string> SnapshotRecentHistory()
    {
        lock (ContextLock)
        {
            return RecentNormalizedHistory.ToArray();
        }
    }

    private static void RememberContext(string source)
    {
        var normalized = NormalizeKey(source);
        if (normalized.Length == 0)
        {
            return;
        }

        lock (ContextLock)
        {
            RecentNormalizedHistory.Add(normalized);
            if (RecentNormalizedHistory.Count > RecentHistoryLimit)
            {
                RecentNormalizedHistory.RemoveAt(0);
            }
        }
    }

    private static bool EnterPatchedCall()
    {
        if (_patchReentryDepth > 0)
        {
            return false;
        }

        _patchReentryDepth++;
        return true;
    }

    private static void ExitPatchedCall()
    {
        if (_patchReentryDepth > 0)
        {
            _patchReentryDepth--;
        }
    }

    private static void CaptureAllText(string source, string origin)
    {
        if (!_fullCaptureEnabled || string.IsNullOrEmpty(source))
        {
            return;
        }

        lock (StateLock)
        {
            var key = origin + "\t" + source;
            if (!FullCaptureSeen.Add(key))
            {
                return;
            }

            FullCaptureBuffer.Add(new
            {
                text = source,
                origin,
                has_tags = source.Contains('<'),
                length = source.Length,
                dialog_source = _currentDialogSourceFile ?? ""
            });

            _fullCaptureFlushCount++;
            if (_fullCaptureFlushCount % 50 == 0)
            {
                FlushFullCaptureUnsafe();
            }
        }
    }

    private static void FlushFullCaptureUnsafe()
    {
        try
        {
            if (string.IsNullOrWhiteSpace(_fullCapturePath))
            {
                return;
            }

            var payload = new
            {
                generated_at = DateTime.Now.ToString("s"),
                count = FullCaptureBuffer.Count,
                entries = FullCaptureBuffer.ToArray()
            };

            RunWithSuppressedDiagnostics(() =>
                File.WriteAllText(_fullCapturePath, JsonSerializer.Serialize(payload, new JsonSerializerOptions
                {
                    WriteIndented = true
                })));
        }
        catch (Exception ex)
        {
            LogSource?.LogWarning($"Failed to write full capture: {ex.Message}");
        }
    }

    private static void CaptureUntranslated(string source)
    {
        if (_suppressDiagnostics)
        {
            return;
        }

        if (string.IsNullOrWhiteSpace(source))
        {
            return;
        }

        var text = NormalizeKey(source);
        if (text.Length < 8)
        {
            return;
        }
        if (!Regex.IsMatch(text, "[A-Za-z]") || !text.Contains(' '))
        {
            return;
        }
        if (TranslationMap.ContainsKey(source) || NormalizedMap.ContainsKey(text))
        {
            return;
        }

        lock (StateLock)
        {
            if (!UntranslatedSeen.Add(text))
            {
                return;
            }

            _newCaptureCount++;
            if (_newCaptureCount % 20 == 0)
            {
                FlushCaptureUnsafe();
            }
        }
    }

    private static void FlushCaptureUnsafe()
    {
        try
        {
            if (string.IsNullOrWhiteSpace(_capturePath))
            {
                return;
            }

            var items = UntranslatedSeen
                .OrderBy(x => x, StringComparer.Ordinal)
                .Select(s => new
                {
                    source = s,
                    target = "",
                    status = "new",
                    category = "runtime_capture",
                    source_file = "runtime",
                    tags = new[] { "runtime_missing" }
                })
                .ToArray();

            var payload = new
            {
                generated_at = DateTime.Now.ToString("s"),
                count = items.Length,
                entries = items
            };

            RunWithSuppressedDiagnostics(() =>
                File.WriteAllText(_capturePath, JsonSerializer.Serialize(payload, new JsonSerializerOptions
                {
                    WriteIndented = true
                })));
        }
        catch (Exception ex)
        {
            LogSource?.LogWarning($"Failed to write untranslated capture: {ex.Message}");
        }
    }

    private static void CaptureChoice(string source)
    {
        if (_suppressDiagnostics || string.IsNullOrWhiteSpace(source) || string.IsNullOrWhiteSpace(_choiceCapturePath))
        {
            return;
        }

        var normalized = NormalizeChoiceText(source);
        var category = ClassifyChoice(source, normalized);

        var captureKey = $"{category}|{normalized}";
        lock (StateLock)
        {
            if (!ChoiceCaptureSeen.Add(captureKey))
            {
                return;
            }

            _choiceCaptureCount++;
            switch (category)
            {
                case "internal_branch_like":
                    _choiceInternalBranchCount++;
                    break;
                case "template_like":
                    _choiceTemplateCount++;
                    break;
                case "stat_gate_choice":
                    _choiceStatGateCount++;
                    break;
                case "short_result_like":
                    _choiceShortResultCount++;
                    break;
                default:
                    _choiceNormalCount++;
                    break;
            }

            FlushChoiceCaptureUnsafe();
            WriteStateUnsafe("choice_capture");
        }
    }

    private static string ClassifyChoice(string source, string normalized)
    {
        var trimmed = normalized;
        if (IsShortResultChoice(trimmed))
        {
            return "short_result_like";
        }

        if (IsInternalLabelChoice(trimmed) ||
            trimmed.Contains("Show Quest Branch", StringComparison.Ordinal) ||
            trimmed.Contains("Show Unlocked Feat", StringComparison.Ordinal) ||
            trimmed.Contains("CHOOSING VARIABLE ROUTE", StringComparison.Ordinal))
        {
            return "internal_branch_like";
        }

        if (trimmed.Contains("SpellName", StringComparison.Ordinal) ||
            trimmed.Contains("Whatever", StringComparison.Ordinal) ||
            trimmed.Contains("Attribute", StringComparison.Ordinal) ||
            trimmed.Contains("Reason for getting", StringComparison.Ordinal))
        {
            return "template_like";
        }

        if (Regex.IsMatch(trimmed, @"^\[[^\]]+\]", RegexOptions.CultureInvariant))
        {
            return "stat_gate_choice";
        }

        return "normal_choice";
    }

    private static string NormalizeChoiceText(string source)
    {
        var normalized = NormalizeKey(source);
        if (normalized.Length > 0)
        {
            return normalized;
        }

        return source.Trim();
    }

    private static bool IsShortResultChoice(string trimmed)
    {
        return trimmed == "S" || trimmed == "F";
    }

    private static bool IsInternalLabelChoice(string trimmed)
    {
        return trimmed.Length >= 3 &&
               trimmed.StartsWith("-", StringComparison.Ordinal) &&
               trimmed.EndsWith("-", StringComparison.Ordinal);
    }

    private static void FlushChoiceCaptureUnsafe()
    {
        try
        {
            if (string.IsNullOrWhiteSpace(_choiceCapturePath))
            {
                return;
            }

            var items = ChoiceCaptureSeen
                .OrderBy(x => x, StringComparer.Ordinal)
                .Select(key =>
                {
                    var split = key.IndexOf('|');
                    var category = split >= 0 ? key.Substring(0, split) : "unknown";
                    var source = split >= 0 ? key[(split + 1)..] : key;
                    return new
                    {
                        source,
                        category,
                        target = "",
                        status = "new",
                        source_file = _currentDialogSourceFile ?? "runtime",
                        tags = new[] { "choice_capture", category }
                    };
                })
                .ToArray();

            var payload = new
            {
                generated_at = DateTime.Now.ToString("s"),
                count = items.Length,
                entries = items
            };

            RunWithSuppressedDiagnostics(() =>
                File.WriteAllText(_choiceCapturePath, JsonSerializer.Serialize(payload, new JsonSerializerOptions
                {
                    WriteIndented = true
                })));
        }
        catch (Exception ex)
        {
            LogSource?.LogWarning($"Failed to write choice capture: {ex.Message}");
        }
    }

    private static void CaptureUiToolkitText(object? instance, string value)
    {
        if (_suppressDiagnostics || string.IsNullOrWhiteSpace(_uiToolkitDumpPath) || string.IsNullOrWhiteSpace(value))
        {
            return;
        }

        try
        {
            var path = GetUiToolkitElementPath(instance);
            var typeName = instance?.GetType().FullName ?? string.Empty;
            var key = $"{typeName}|{path}|{value}";

            lock (StateLock)
            {
                if (!DumpedUiToolkitEntries.Add(key))
                {
                    return;
                }
            }

            var line = $"{DateTime.Now:s}`t{typeName}`t{path}`t{value}{Environment.NewLine}";
            RunWithSuppressedDiagnostics(() => File.AppendAllText(_uiToolkitDumpPath!, line));
        }
        catch
        {
        }
    }

    private static string GetUiToolkitElementPath(object? instance)
    {
        if (instance is null)
        {
            return string.Empty;
        }

        try
        {
            var names = new List<string>();
            object? current = instance;
            while (current is not null)
            {
                var name = ExtractUnityObjectName(current);
                if (string.IsNullOrWhiteSpace(name))
                {
                    name = ReadStringProperty(current, "name");
                }

                if (!string.IsNullOrWhiteSpace(name))
                {
                    names.Add(name);
                }
                else
                {
                    names.Add(current.GetType().Name);
                }

                var parentProp = current.GetType().GetProperty("parent", BindingFlags.Instance | BindingFlags.Public | BindingFlags.NonPublic);
                current = parentProp?.GetValue(current);
            }

            names.Reverse();
            return string.Join("/", names);
        }
        catch
        {
            return string.Empty;
        }
    }

    private static void RecordTranslationHit(string source, string target)
    {
        lock (StateLock)
        {
            _translateHitCount++;
            _lastTranslatedSource = source;
            _lastTranslatedTarget = target;

            if (_fullCaptureEnabled && !string.IsNullOrEmpty(source) && !string.IsNullOrEmpty(target))
            {
                _translationHitLog.Add(new { source, target });
                _translationHitLogFlushCount++;
                if (_translationHitLogFlushCount % 100 == 0)
                {
                    FlushTranslationHitLog();
                }
            }
        }
    }

    private static void FlushTranslationHitLog()
    {
        try
        {
            if (string.IsNullOrWhiteSpace(_translationHitLogPath) || _translationHitLog.Count == 0)
                return;

            var payload = new
            {
                generated_at = DateTime.Now.ToString("yyyy-MM-ddTHH:mm:ss"),
                count = _translationHitLog.Count,
                entries = _translationHitLog
            };
            File.WriteAllText(_translationHitLogPath,
                JsonSerializer.Serialize(payload, new JsonSerializerOptions { WriteIndented = true }),
                System.Text.Encoding.UTF8);
        }
        catch { /* silent */ }
    }

    private static void RecordTranslationMiss(string source)
    {
        lock (StateLock)
        {
            _translateMissCount++;
            _lastMissedSource = source;
        }
    }

    private static void WriteState(string phase)
    {
        lock (StateLock)
        {
            WriteStateUnsafe(phase);
            FlushTranslationHitLog();
        }
    }

    private static void WriteStateUnsafe(string phase)
    {
        try
        {
            if (string.IsNullOrWhiteSpace(_statePath))
            {
                return;
            }

            var payload = new
            {
                phase,
                plugin_version = PluginVersion,
                written_at = DateTime.Now.ToString("s"),
                translations_loaded = TranslationMap.Count,
                normalized_loaded = NormalizedMap.Count,
                contextual_loaded = _contextualLoadedCount,
                contextual_hits = _contextualHitCount,
                text_asset_overrides_loaded = _textAssetOverrideCount,
                text_asset_override_hits = _textAssetOverrideHitCount,
                text_asset_override_miss_logged = _textAssetOverrideMissLoggedCount,
                localization_id_overrides_loaded = _localizationIdOverrideCount,
                localization_id_override_hits = _localizationIdOverrideHitCount,
                localization_id_override_miss_logged = _localizationIdOverrideMissLoggedCount,
                runtime_lexicon_substrings = _runtimeLexiconSubstringCount,
                runtime_lexicon_regex_rules = _runtimeLexiconRegexCount,
                runtime_lexicon_hits = _runtimeLexiconHitCount,
                menu_scene_sweeps = _menuSceneSweepCount,
                menu_scene_translated = _menuSceneTranslatedCount,
                menu_scene_samples = _menuSweepSampleCount,
                menu_direct_override_hits = _menuDirectOverrideHits,
                scene_text_scans = _sceneTextScanCount,
                choice_capture_count = _choiceCaptureCount,
                choice_internal_branch_like = _choiceInternalBranchCount,
                choice_template_like = _choiceTemplateCount,
                choice_stat_gate = _choiceStatGateCount,
                choice_short_result_like = _choiceShortResultCount,
                choice_normal = _choiceNormalCount,
                source_file_context_hits = _sourceFileContextHitCount,
                tmp_patched = InstanceState?.TmpPatched,
                text_asset_patched = InstanceState?.TextAssetPatched,
                localization_manager_patched = InstanceState?.LocalizationManagerPatched,
                ui_patched = InstanceState?.UiPatched,
                menu_patched = InstanceState?.MenuPatched,
                dialog_patched = InstanceState?.DialogPatched,
                translation_hits = _translateHitCount,
                translation_misses = _translateMissCount,
                current_dialog_source_file = _currentDialogSourceFile ?? string.Empty,
                last_translated_source = _lastTranslatedSource,
                last_translated_target = _lastTranslatedTarget,
                last_missed_source = _lastMissedSource,
                font_status = _fontStatus,
                font_fallback_injected = _fontFallbackInjectedCount
            };

            RunWithSuppressedDiagnostics(() =>
                File.WriteAllText(_statePath, JsonSerializer.Serialize(payload, new JsonSerializerOptions
                {
                    WriteIndented = true
                })));
        }
        catch
        {
        }
    }

    private static void RunWithSuppressedDiagnostics(Action action)
    {
        var previous = _suppressDiagnostics;
        _suppressDiagnostics = true;
        try
        {
            action();
        }
        finally
        {
            _suppressDiagnostics = previous;
        }
    }

    private static PluginState? InstanceState { get; set; }


    private static string StripGameplayPrefix(string text)
    {
        if (string.IsNullOrWhiteSpace(text))
        {
            return string.Empty;
        }

        text = Regex.Replace(text, @"^(?:DC|FC)\d+\s+[A-Za-z]+-\s*", "", RegexOptions.CultureInvariant);
        text = Regex.Replace(text, @"^(?:OBJ|reply|wis|int|str|dex|con|cha|buy|sell|roll)\s*[:\-]\s*", "", RegexOptions.IgnoreCase | RegexOptions.CultureInvariant);
        return text.Trim();
    }

    private static string StripOuterQuotes(string text)
    {
        if (string.IsNullOrWhiteSpace(text))
        {
            return string.Empty;
        }

        text = text.Trim();
        return text.Length >= 2 && text[0] == '"' && text[^1] == '"'
            ? text.Substring(1, text.Length - 2).Trim()
            : text;
    }

    private static string StripOuterFormattingTags(string text)
    {
        if (string.IsNullOrWhiteSpace(text))
        {
            return string.Empty;
        }

        text = text.Trim();
        while (TryStripOuterFormattingTag(text, out _, out var innerText, out _))
        {
            text = innerText.Trim();
        }

        return text;
    }





    private static bool TryStripOuterFormattingTag(string text, out string openTag, out string innerText, out string closeTag)
    {
        openTag = string.Empty;
        innerText = text;
        closeTag = string.Empty;

        var match = Regex.Match(
            text,
            @"^(<(?<name>[A-Za-z][A-Za-z0-9-]*)(?:=[^>]*)?>)(?<inner>.*?)(</\k<name>>)$",
            RegexOptions.Singleline | RegexOptions.CultureInvariant);

        if (!match.Success)
        {
            return false;
        }

        openTag = match.Groups[1].Value;
        innerText = match.Groups["inner"].Value;
        closeTag = match.Groups[4].Value;
        return true;
    }

    private static Type? FindTypeByName(string typeName)
    {
        lock (StateLock)
        {
            if (TypeCache.TryGetValue(typeName, out var cached))
            {
                return cached;
            }
        }

        Type? resolved = null;
        foreach (var asm in AppDomain.CurrentDomain.GetAssemblies())
        {
            var type = asm.GetType(typeName, throwOnError: false);
            if (type is not null)
            {
                resolved = type;
                break;
            }
        }

        lock (StateLock)
        {
            TypeCache[typeName] = resolved;
        }

        return resolved;
    }

    private sealed class PluginState
    {
        public bool TmpPatched { get; init; }
        public bool TextAssetPatched { get; init; }
        public bool LocalizationManagerPatched { get; init; }
        public bool UiPatched { get; init; }
        public bool MenuPatched { get; init; }
        public bool DialogPatched { get; init; }
    }

    private sealed class ContextualEntry
    {
        public string Id { get; init; } = string.Empty;
        public string Source { get; init; } = string.Empty;
        public string Target { get; init; } = string.Empty;
        public string ContextEn { get; init; } = string.Empty;
        public string SpeakerHint { get; init; } = string.Empty;
        public string TextRole { get; init; } = string.Empty;
        public string TranslationLane { get; init; } = string.Empty;
        public string SourceFile { get; init; } = string.Empty;
    }



    private sealed class RuntimeRegexRule
    {
        public string Name { get; init; } = string.Empty;
        public Regex Regex { get; init; } = null!;
        public string Replace { get; init; } = string.Empty;
    }

}
