import argparse
import csv
import io
import json
import re
import shutil
import sqlite3
import subprocess
from collections import Counter, defaultdict
from pathlib import Path

from build_textasset_overrides import generate_textasset_overrides


TOKEN_RE = re.compile(r"(\$[A-Za-z0-9_]+|<[^>]+>|\{[^{}]+\})")
HTML_TAG_RE = re.compile(r"<[^>]+>")
DB_MARKUP_RE = re.compile(r"\[\[/?E\d+\]\]")
WS_RE = re.compile(r"\s+")


def load_json(path: Path):
    return json.loads(path.read_text(encoding="utf-8-sig"))


def write_json(path: Path, payload):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8")


def token_compatible(source: str, target: str) -> bool:
    src_tokens = TOKEN_RE.findall(source)
    tgt_tokens = TOKEN_RE.findall(target)
    if src_tokens != tgt_tokens:
        return False
    return source.count("\n") == target.count("\n")


def collect_done_rows_sqlite(db_path: Path, pipeline_version: str):
    con = sqlite3.connect(str(db_path))
    cur = con.cursor()
    rows = cur.execute(
        "select id, pack_json, updated_at from items where status='done' and pack_json is not null and pack_json<>''"
    ).fetchall()
    con.close()

    out = []
    for item_id, pack_json, updated_at in rows:
        try:
            pack = json.loads(pack_json)
        except json.JSONDecodeError:
            continue
        if pipeline_version and pack.get("pipeline_version") != pipeline_version:
            continue
        target = (pack.get("proposed_ko_restored") or "").strip()
        if not item_id or not target:
            continue
        out.append(
            {
                "id": item_id,
                "target": target,
                "updated_at": updated_at or "",
                "pack": pack,
            }
        )
    return out


def collect_done_rows_postgres(dsn: str, pipeline_version: str, psql_path: str):
    sql = """
COPY (
    SELECT i.id, i.pack_json::text, i.updated_at::text
    FROM items i
    JOIN pipeline_items p ON p.id = i.id
    WHERE p.state = 'done'
      AND i.pack_json IS NOT NULL
      AND COALESCE(i.pack_json->>'proposed_ko_restored', '') <> ''
) TO STDOUT WITH CSV
"""
    proc = subprocess.run(
        [psql_path, dsn, "-q", "-c", sql],
        capture_output=True,
        text=True,
        encoding="utf-8",
        check=False,
    )
    if proc.returncode != 0:
        raise RuntimeError(proc.stderr.strip() or "psql query failed")

    out = []
    reader = csv.reader(io.StringIO(proc.stdout))
    for row in reader:
        if len(row) != 3:
            continue
        item_id, pack_json, updated_at = row
        try:
            pack = json.loads(pack_json)
        except json.JSONDecodeError:
            continue
        if pipeline_version and pack.get("pipeline_version") != pipeline_version:
            continue
        target = (pack.get("proposed_ko_restored") or "").strip()
        if not item_id or not target:
            continue
        out.append(
            {
                "id": item_id,
                "target": target,
                "updated_at": updated_at or "",
                "pack": pack,
            }
        )
    return out


def collect_done_rows(db_backend: str, db_ref: str, pipeline_version: str, psql_path: str):
    if db_backend == "postgres":
        return collect_done_rows_postgres(db_ref, pipeline_version, psql_path)
    return collect_done_rows_sqlite(Path(db_ref), pipeline_version)


def first_non_empty(*values):
    for value in values:
        if isinstance(value, str) and value.strip():
            return value.strip()
    return ""


def normalize_text(value: str) -> str:
    value = HTML_TAG_RE.sub("", value or "")
    value = DB_MARKUP_RE.sub("", value)
    value = value.replace("\r\n", "\n").replace("\r", "\n")
    value = WS_RE.sub(" ", value).strip()
    return value


def normalize_optional(value):
    if not isinstance(value, str):
        return ""
    return normalize_text(value)


def build_runtime_indexes(path: Path | None):
    if path is None or not path.exists():
        return {}, {}

    payload = load_json(path)
    if not isinstance(payload, list):
        return {}, {}

    by_id = {}
    by_source = defaultdict(list)
    for item in payload:
        if not isinstance(item, dict):
            continue
        line_id = item.get("line_id")
        if not isinstance(line_id, str) or not line_id:
            continue
        by_id[line_id] = item
        source_key = normalize_optional(item.get("source_text"))
        if source_key:
            by_source[source_key].append(item)
    return by_id, by_source


def context_alignment_score(context_en: str, candidate: dict, runtime_by_id: dict[str, dict]) -> int:
    context = normalize_optional(context_en)
    if not context:
        return 0

    context_lines = [line for line in context.split("\n") if line]
    if not context_lines:
        return 0

    current = normalize_optional(candidate.get("source_text"))
    prev_line = runtime_by_id.get(candidate.get("prev_line_id"))
    next_line = runtime_by_id.get(candidate.get("next_line_id"))
    prev_text = normalize_optional(prev_line.get("source_text")) if isinstance(prev_line, dict) else ""
    next_text = normalize_optional(next_line.get("source_text")) if isinstance(next_line, dict) else ""

    best = 0
    for index, line in enumerate(context_lines):
        if line != current:
            continue
        score = 1
        if prev_text and index > 0 and context_lines[index - 1] == prev_text:
            score += 2
        if next_text and index + 1 < len(context_lines) and context_lines[index + 1] == next_text:
            score += 2
        best = max(best, score)
    return best


def score_runtime_candidate(source_text: str, pack: dict, candidate: dict, runtime_by_id: dict[str, dict]) -> int:
    score = 0
    if normalize_optional(source_text) == normalize_optional(candidate.get("source_text")):
        score += 3

    pack_role = normalize_optional(pack.get("text_role"))
    candidate_role = normalize_optional(candidate.get("text_role"))
    if pack_role and pack_role == candidate_role:
        score += 2

    pack_speaker = normalize_optional(pack.get("speaker_hint"))
    candidate_speaker = normalize_optional(candidate.get("speaker_hint"))
    if pack_speaker and pack_speaker == candidate_speaker:
        score += 3
    elif not pack_speaker and not candidate_speaker:
        score += 1

    score += context_alignment_score(pack.get("context_en", ""), candidate, runtime_by_id)
    return score


def infer_runtime_metadata(source_text: str, row: dict, runtime_by_id: dict[str, dict], runtime_by_source: dict[str, list[dict]]):
    candidates = runtime_by_source.get(normalize_optional(source_text), [])
    if not candidates:
        return {}

    scored = []
    for candidate in candidates:
        score = score_runtime_candidate(source_text, row["pack"], candidate, runtime_by_id)
        if score > 0:
            scored.append((score, candidate))

    if not scored:
        return {}

    scored.sort(key=lambda item: item[0], reverse=True)
    best_score = scored[0][0]
    best_candidates = [candidate for score, candidate in scored if score == best_score]
    if len(best_candidates) != 1:
        return {}

    return best_candidates[0]


def build_context_entry(item_id: str, source_text: str, target_text: str, row, runtime_by_id, runtime_by_source):
    pack = row["pack"]
    meta = runtime_by_id.get(item_id, {})
    if not meta:
        meta = infer_runtime_metadata(source_text, row, runtime_by_id, runtime_by_source)
    return {
        "id": item_id,
        "source": source_text,
        "target": target_text,
        "status": "translated",
        "source_raw": first_non_empty(pack.get("source_raw"), source_text),
        "context_en": first_non_empty(pack.get("context_en"), source_text),
        "speaker_hint": first_non_empty(pack.get("speaker_hint")),
        "text_role": first_non_empty(pack.get("text_role")),
        "choice_prefix": first_non_empty(pack.get("choice_prefix")),
        "translation_lane": first_non_empty(pack.get("translation_lane")),
        "risk": first_non_empty(pack.get("risk")),
        "source_file": first_non_empty(meta.get("source_file")),
        "meta_path_label": first_non_empty(meta.get("meta_path_label")),
        "prev_line_id": first_non_empty(meta.get("prev_line_id")),
        "next_line_id": first_non_empty(meta.get("next_line_id")),
        "updated_at": row["updated_at"],
    }


def build_outputs(source_strings, current_strings, done_rows, runtime_by_id, runtime_by_source):
    applied_strings = {"strings": {}}
    translated_by_id = {}
    contextual_entries = []
    not_found_ids = []
    token_rejected_ids = []

    source_to_targets = defaultdict(list)

    for item_id, current_row in current_strings.items():
        current_text = ""
        if isinstance(current_row, dict):
            current_text = current_row.get("Text", "")
        applied_strings["strings"][item_id] = {"Text": current_text}

    for row in done_rows:
        item_id = row["id"]
        source_row = source_strings.get(item_id)
        if not isinstance(source_row, dict):
            not_found_ids.append(item_id)
            continue

        source_text = source_row.get("Text", "")
        target_text = row["target"]
        if not token_compatible(source_text, target_text):
            token_rejected_ids.append(item_id)
            continue

        translated_by_id[item_id] = {
            "source": source_text,
            "target": target_text,
            "updated_at": row["updated_at"],
        }
        contextual_entries.append(build_context_entry(item_id, source_text, target_text, row, runtime_by_id, runtime_by_source))
        applied_strings["strings"][item_id] = {"Text": target_text}
        source_to_targets[source_text].append((target_text, item_id, row))

    sidecar_entries = []
    conflict_report = []
    ambiguous_source_count = 0

    for source_text, pairs in source_to_targets.items():
        variants = Counter(target for target, _, _ in pairs)
        if len(variants) > 1:
            ambiguous_source_count += 1
            conflict_report.append(
                {
                    "source": source_text,
                    "variant_count": len(variants),
                    "total_occurrences": len(pairs),
                    "variants": [
                        {
                            "target": target,
                            "count": count,
                            "sample_ids": [item_id for tgt, item_id, _ in pairs if tgt == target][:5],
                            "sample_contexts": [
                                {
                                    "id": item_id,
                                    "context_en": first_non_empty(row["pack"].get("context_en"), source_text),
                                    "speaker_hint": first_non_empty(row["pack"].get("speaker_hint")),
                                    "text_role": first_non_empty(row["pack"].get("text_role")),
                                }
                                for tgt, item_id, row in pairs
                                if tgt == target
                            ][:3],
                        }
                        for target, count in variants.most_common()
                    ],
                }
            )
            continue

        target_text = pairs[0][0]
        chosen_row = pairs[0][2]
        sidecar_entries.append(
            {
                "source": source_text,
                "target": target_text,
                "status": "translated",
                "text_role": first_non_empty(chosen_row["pack"].get("text_role")),
                "speaker_hint": first_non_empty(chosen_row["pack"].get("speaker_hint")),
                "translation_lane": first_non_empty(chosen_row["pack"].get("translation_lane")),
            }
        )

    return {
        "applied_strings": applied_strings,
        "translated_by_id": translated_by_id,
        "contextual_entries": contextual_entries,
        "sidecar_entries": sidecar_entries,
        "conflict_report": sorted(conflict_report, key=lambda x: x["variant_count"], reverse=True),
        "not_found_ids": not_found_ids,
        "token_rejected_ids": token_rejected_ids,
        "ambiguous_source_count": ambiguous_source_count,
    }


def build_distribution(dist_dir: Path, plugin_dll: Path, sidecar_payload, report_payload):
    plugin_target = dist_dir / "BepInEx" / "plugins" / "EsotericEbbTranslationLoader" / plugin_dll.name
    sidecar_target = dist_dir / "Esoteric Ebb_Data" / "StreamingAssets" / "TranslationPatch" / "translations.json"
    report_target = dist_dir / "Esoteric Ebb_Data" / "StreamingAssets" / "TranslationPatch" / "build_report.json"
    runtime_lexicon_target = dist_dir / "Esoteric Ebb_Data" / "StreamingAssets" / "TranslationPatch" / "runtime_lexicon.json"
    fonts_target_dir = dist_dir / "Esoteric Ebb_Data" / "StreamingAssets" / "TranslationPatch" / "fonts"
    localizationtexts_target_dir = dist_dir / "Esoteric Ebb_Data" / "StreamingAssets" / "TranslationPatch" / "localizationtexts"
    textassets_target_dir = dist_dir / "Esoteric Ebb_Data" / "StreamingAssets" / "TranslationPatch" / "textassets"
    install_script = dist_dir / "install_patch.ps1"
    readme = dist_dir / "README.txt"
    assets_font_dir = Path(__file__).resolve().parent.parent / "assets" / "fonts"
    static_localization_dir = Path(__file__).resolve().parent.parent / "output" / "static_reinject" / "localizationtexts_ko"
    runtime_lexicon_source = Path(__file__).resolve().parent.parent / "input" / "runtime_lexicon.json"
    generated_textasset_override_dir = dist_dir.parent / "artifacts" / "textasset_overrides"

    plugin_target.parent.mkdir(parents=True, exist_ok=True)
    sidecar_target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(plugin_dll, plugin_target)
    write_json(sidecar_target, sidecar_payload)
    write_json(report_target, report_payload)
    if runtime_lexicon_source.exists():
        runtime_lexicon_target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(runtime_lexicon_source, runtime_lexicon_target)
    if assets_font_dir.exists():
        fonts_target_dir.mkdir(parents=True, exist_ok=True)
        for item in assets_font_dir.iterdir():
            if item.is_file():
                shutil.copy2(item, fonts_target_dir / item.name)
    if static_localization_dir.exists():
        localizationtexts_target_dir.mkdir(parents=True, exist_ok=True)
        for item in static_localization_dir.iterdir():
            if item.is_file() and item.suffix.lower() == ".txt":
                shutil.copy2(item, localizationtexts_target_dir / item.name)
    if generated_textasset_override_dir.exists():
        textassets_target_dir.mkdir(parents=True, exist_ok=True)
        for item in generated_textasset_override_dir.iterdir():
            if item.is_file() and item.suffix.lower() == ".txt":
                shutil.copy2(item, textassets_target_dir / item.name)

    install_script.write_text(
        """param(
    [string]$GameDir = "."
)

$ErrorActionPreference = "Stop"
$patchRoot = Split-Path -Parent $MyInvocation.MyCommand.Path

Copy-Item -Path (Join-Path $patchRoot "BepInEx") -Destination $GameDir -Recurse -Force
Copy-Item -Path (Join-Path $patchRoot "Esoteric Ebb_Data") -Destination $GameDir -Recurse -Force

Write-Host "Patch installed to $GameDir"
""",
        encoding="utf-8",
    )

    readme.write_text(
        """Esoteric Ebb Korean Patch

Install
1. Extract this package into the game folder, or run install_patch.ps1 with the game folder path.
2. Required result files:
   - BepInEx\\plugins\\EsotericEbbTranslationLoader\\EsotericEbb.TranslationLoader.dll
   - Esoteric Ebb_Data\\StreamingAssets\\TranslationPatch\\translations.json
   - Esoteric Ebb_Data\\StreamingAssets\\TranslationPatch\\runtime_lexicon.json
   - Esoteric Ebb_Data\\StreamingAssets\\TranslationPatch\\fonts\\NotoSansCJKkr-Regular.otf
   - Esoteric Ebb_Data\\StreamingAssets\\TranslationPatch\\localizationtexts\\UIElements.txt
   - Esoteric Ebb_Data\\StreamingAssets\\TranslationPatch\\textassets\\LL_ZombieCheck.txt
3. Run the game once.

Notes
- This package excludes source-text conflicts that cannot be resolved safely by the runtime loader.
- This package includes a bundled Korean font fallback for improved readability.
- This package can override static localization text tables such as menu/UI CSV resources.
- This package can also override extracted Ink/TextAsset JSON payloads by asset key.
- Check BepInEx\\LogOutput.log after launch.
""",
        encoding="utf-8",
    )


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--db", required=True)
    parser.add_argument("--db-backend", choices=["sqlite", "postgres"], default="sqlite")
    parser.add_argument("--source", required=True)
    parser.add_argument("--current", required=True)
    parser.add_argument("--plugin-dll", required=True)
    parser.add_argument("--pipeline-version", default="chunkctx-v1")
    parser.add_argument("--out-dir", required=True)
    parser.add_argument("--line-meta", default="")
    parser.add_argument("--psql-path", default=r"C:\Program Files\PostgreSQL\17\bin\psql.exe")
    args = parser.parse_args()

    source_path = Path(args.source)
    current_path = Path(args.current)
    plugin_dll = Path(args.plugin_dll)
    out_dir = Path(args.out_dir)

    source_root = load_json(source_path)
    current_root = load_json(current_path)
    source_strings = source_root.get("strings", {})
    current_strings = current_root.get("strings", {})
    done_rows = collect_done_rows(args.db_backend, args.db, args.pipeline_version, args.psql_path)
    runtime_by_id, runtime_by_source = build_runtime_indexes(Path(args.line_meta)) if args.line_meta else ({}, {})

    result = build_outputs(source_strings, current_strings, done_rows, runtime_by_id, runtime_by_source)

    artifacts_dir = out_dir / "artifacts"
    dist_dir = out_dir / "dist"
    artifacts_dir.mkdir(parents=True, exist_ok=True)
    dist_dir.mkdir(parents=True, exist_ok=True)

    applied_strings_path = artifacts_dir / "current_esoteric.translated.json"
    sidecar_path = artifacts_dir / "translations.json"
    conflict_path = artifacts_dir / "translation_conflicts.json"
    by_id_path = artifacts_dir / "translation_by_id.json"
    contextual_path = artifacts_dir / "translation_contextual.json"
    report_path = artifacts_dir / "build_report.json"
    textasset_overrides_dir = artifacts_dir / "textasset_overrides"
    textasset_overrides_report_path = artifacts_dir / "textasset_overrides_report.json"

    sidecar_payload = {
        "format": "esoteric-ebb-sidecar.v3",
        "entries": result["sidecar_entries"],
        "contextual_entries": result["contextual_entries"],
        "metadata": {
            "db": str(args.db),
            "db_backend": args.db_backend,
            "source": str(source_path),
            "current": str(current_path),
            "pipeline_version": args.pipeline_version,
            "line_meta": str(Path(args.line_meta)) if args.line_meta else "",
        },
    }
    report_payload = {
        "db": str(args.db),
        "db_backend": args.db_backend,
        "source": str(source_path),
        "current": str(current_path),
        "pipeline_version": args.pipeline_version,
        "done_rows": len(done_rows),
        "applied_line_ids": len(result["translated_by_id"]),
        "sidecar_entries": len(result["sidecar_entries"]),
        "contextual_entries": len(result["contextual_entries"]),
        "ambiguous_source_count": result["ambiguous_source_count"],
        "token_rejected_count": len(result["token_rejected_ids"]),
        "not_found_count": len(result["not_found_ids"]),
    }

    extract_textasset_dir = Path(__file__).resolve().parent.parent.parent / "extract" / "ExportedProject" / "Assets" / "TextAsset"
    textasset_override_report = generate_textasset_overrides(
        extract_textasset_dir,
        {line_id: row["target"] for line_id, row in result["translated_by_id"].items()},
        textasset_overrides_dir,
    )
    report_payload["textasset_override_files"] = textasset_override_report["override_files_written"]
    report_payload["textasset_override_translated_lines"] = textasset_override_report["translated_lines"]

    write_json(applied_strings_path, result["applied_strings"])
    write_json(sidecar_path, sidecar_payload)
    write_json(conflict_path, result["conflict_report"])
    write_json(by_id_path, result["translated_by_id"])
    write_json(contextual_path, result["contextual_entries"])
    write_json(report_path, report_payload)
    write_json(textasset_overrides_report_path, textasset_override_report)
    build_distribution(dist_dir, plugin_dll, sidecar_payload, report_payload)

    print(f"applied_strings={applied_strings_path}")
    print(f"sidecar={sidecar_path}")
    print(f"conflicts={conflict_path}")
    print(f"dist={dist_dir}")
    print(json.dumps(report_payload, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
