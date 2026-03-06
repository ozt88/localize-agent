# Pipeline Ops

## Active Project

- Project: `esoteric-ebb`
- Canonical source: `projects/esoteric-ebb/source/translation_assetripper_textasset_unique.json`
- Active batch dir: `projects/esoteric-ebb/output/batches/translation_assetripper_textasset_unique`

## Command Flow

1. Adapt the unified source into workflow input files.
2. Run translation with `--project esoteric-ebb`.
3. Run evaluation with `--project esoteric-ebb`.
4. Apply translated rows back into the unified source schema.

## Commands

### Adapt input
```powershell
powershell -ExecutionPolicy Bypass -File .\projects\esoteric-ebb\cmd\prepare_input.ps1
```

### Translate
```powershell
powershell -ExecutionPolicy Bypass -File .\projects\esoteric-ebb\cmd\run_translate.ps1
```

### Evaluate
```powershell
powershell -ExecutionPolicy Bypass -File .\projects\esoteric-ebb\cmd\run_evaluate.ps1
```

### Apply output
```powershell
powershell -ExecutionPolicy Bypass -File .\projects\esoteric-ebb\cmd\apply_output.ps1
```

## Expected Files

- Source JSON: `projects/esoteric-ebb/source/translation_assetripper_textasset_unique.json`
- Adapted source JSON: `projects/esoteric-ebb/output/batches/translation_assetripper_textasset_unique/source_esoteric.json`
- Adapted current JSON: `projects/esoteric-ebb/output/batches/translation_assetripper_textasset_unique/current_esoteric.json`
- ID list: `projects/esoteric-ebb/output/batches/translation_assetripper_textasset_unique/ids_esoteric.txt`
- Checkpoint DB: `projects/esoteric-ebb/output/batches/translation_assetripper_textasset_unique/translation_checkpoint.db`
- Applied output JSON: `projects/esoteric-ebb/output/batches/translation_assetripper_textasset_unique/translation_assetripper_textasset_unique.translated.json`

## Notes

- `go-translate` and `go-evaluate` should use the project profile in `projects/esoteric-ebb/project.json`.
- Release and local install scripts should use the translated unified output file, not legacy `enGB.json` assets.
