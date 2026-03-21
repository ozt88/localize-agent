python .\projects\rogue-trader\tools\extract_bbp_dialogue_links.py `
  --bbp-file 'E:\SteamLibrary\steamapps\common\Warhammer 40,000 Rogue Trader\Bundles\blueprints-pack.bbp' `
  --out-json .\projects\rogue-trader\source\bbp_dialogue_links.json

python .\projects\rogue-trader\tools\extract_blueprint_bundle_context.py `
  --bundle-file 'E:\SteamLibrary\steamapps\common\Warhammer 40,000 Rogue Trader\Bundles\blueprint.assets' `
  --cheatdata-json 'E:\SteamLibrary\steamapps\common\Warhammer 40,000 Rogue Trader\Bundles\cheatdata.json' `
  --bbp-links-json .\projects\rogue-trader\source\bbp_dialogue_links.json `
  --out-json .\projects\rogue-trader\source\blueprint_bundle_context.json

python .\projects\rogue-trader\tools\extract_scene_bundle_context.py `
  --bundles-root 'E:\SteamLibrary\steamapps\common\Warhammer 40,000 Rogue Trader\Bundles' `
  --cheatdata-json 'E:\SteamLibrary\steamapps\common\Warhammer 40,000 Rogue Trader\Bundles\cheatdata.json' `
  --glob '*mechanics.scenes' `
  --out-json .\projects\rogue-trader\source\scene_bundle_context.json

python .\projects\rogue-trader\tools\build_canonical_translation_source.py `
  --source-json .\projects\rogue-trader\source\enGB_original.json `
  --current-json .\projects\rogue-trader\source\enGB_new.json `
  --glossary-json .\workflow\context\universal_glossary.json `
  --extract-root .\projects\rogue-trader\extract `
  --scene-context-json .\projects\rogue-trader\source\scene_bundle_context.json `
  --blueprint-context-json .\projects\rogue-trader\source\blueprint_bundle_context.json `
  --sound-json .\projects\rogue-trader\extract\ExportedProject\Assets\StreamingAssets\Localization\Sound.json `
  --out-json .\projects\rogue-trader\source\canonical_translation_source.json `
  --out-reference-json .\projects\rogue-trader\source\canonical_translation_reference.json `
  --out-summary .\projects\rogue-trader\source\canonical_translation_summary.json `
  --out-scene-batches-json .\projects\rogue-trader\source\canonical_translation_scene_batches.json
