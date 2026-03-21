#!/usr/bin/env python3
"""
Diff two game extract versions and produce pipeline-ready items for new/changed content.

Compares TextAsset (ink dialogue), TranslationPatch overlay entries, and
localizationtexts CSV files between two extract versions. Uses pipeline_ingest
module for DB insertion and source file updates.

Usage:
    python diff_version_source.py --old 1.1.1 --new 1.1.3 [--output diff_items.json] [--apply]

Steps:
    1. Compare TextAsset .txt files (ink JSON) for new dialogue strings
    2. Compare TranslationPatch/translations.json entries for new overlay items
    3. Enrich overlay items with context from contextual_entries
    4. Compare localizationtexts/ CSV files for new localization strings
    5. Output enriched items JSON
    6. Optionally --apply: ingest into pipeline via pipeline_ingest module
"""

import argparse
import hashlib
import json
import re
import sys
from pathlib import Path

from pipeline_ingest import PipelineIngest, is_translatable

SCRIPT_DIR = Path(__file__).parent
PROJECT_DIR = SCRIPT_DIR.parent
EXTRACT_DIR = PROJECT_DIR / "extract"
BATCH_DIR = PROJECT_DIR / "output" / "batches" / "canonical_full_retranslate_live"

INK_CONTROL_PREFIXES = (".", "->", "#", "/#", "^->")


# ─── TextAsset (ink JSON) ───

def extract_ink_strings(path: Path) -> set[str]:
    """Extract dialogue strings from ink JSON TextAsset."""
    raw = path.read_text(encoding="utf-8-sig")
    strings = re.findall(r'"\^([^"]+?)"', raw)
    filtered = set()
    for s in strings:
        s = s.strip()
        if len(s) <= 2:
            continue
        if any(s.startswith(p) for p in INK_CONTROL_PREFIXES):
            continue
        if re.match(r"^(PlaySFX|FadeTo|PlayMusic|RollLock|StopMusic|StopAmbiance)", s):
            continue
        filtered.add(s)
    return filtered


def diff_textassets(old_dir: Path, new_dir: Path) -> list[dict]:
    """Find new dialogue strings in TextAsset files."""
    old_ta = old_dir / "ExportedProject" / "Assets" / "TextAsset"
    new_ta = new_dir / "ExportedProject" / "Assets" / "TextAsset"
    if not old_ta.exists() or not new_ta.exists():
        return []

    items = []
    for f in sorted(new_ta.glob("*.txt")):
        old_f = old_ta / f.name
        old_strings = extract_ink_strings(old_f) if old_f.exists() else set()
        new_strings = extract_ink_strings(f) - old_strings
        scene = f.stem

        for text in new_strings:
            if not is_translatable(text):
                continue
            h = hashlib.md5((scene + ":" + text).encode("utf-8")).hexdigest()[:12]
            items.append({
                "id": f"line-upd-{scene}-{h}",
                "source": text,
                "text_role": "dialogue",
                "source_type": "textasset_update",
                "source_file": f.name,
                "scene_hint": scene,
            })
    return items


# ─── Overlay (translations.json) ───

def diff_overlay(old_dir: Path, new_dir: Path) -> list[dict]:
    """Find new overlay entries, enriched with context from contextual_entries."""
    old_trans = old_dir / "ExportedProject" / "Assets" / "StreamingAssets" / "TranslationPatch" / "translations.json"
    new_trans = new_dir / "ExportedProject" / "Assets" / "StreamingAssets" / "TranslationPatch" / "translations.json"
    if not old_trans.exists() or not new_trans.exists():
        return []

    old_data = json.loads(old_trans.read_text(encoding="utf-8"))
    new_data = json.loads(new_trans.read_text(encoding="utf-8"))

    old_sources = set(e.get("source", "") for e in old_data["entries"])

    # Build contextual lookup from new version
    ctx_by_source = {}
    ctx_by_id = {}
    for e in new_data.get("contextual_entries", []):
        src = e.get("source", "")
        eid = e.get("id", "")
        if src:
            ctx_by_source[src] = e
        if eid:
            ctx_by_id[eid] = e

    items = []
    for e in new_data["entries"]:
        src = e.get("source", "")
        if not src or src in old_sources:
            continue
        if not is_translatable(src):
            continue

        h = hashlib.md5(src.encode("utf-8")).hexdigest()[:12]
        item = {
            "id": f"ovl-upd-{h}",
            "source": src,
            "text_role": e.get("text_role", "unknown"),
            "source_type": "overlay_update",
        }

        # Enrich with contextual data
        ctx = ctx_by_source.get(src)
        if ctx:
            prev_id = ctx.get("prev_line_id", "")
            next_id = ctx.get("next_line_id", "")
            item.update({
                "context_en": ctx.get("context_en", ""),
                "speaker_hint": ctx.get("speaker_hint", ""),
                "prev_line_id": prev_id,
                "next_line_id": next_id,
                "prev_en": ctx_by_id[prev_id].get("source", "") if prev_id and prev_id in ctx_by_id else "",
                "next_en": ctx_by_id[next_id].get("source", "") if next_id and next_id in ctx_by_id else "",
                "source_file": ctx.get("source_file", ""),
                "meta_path_label": ctx.get("meta_path_label", ""),
            })
        items.append(item)

    return items


# ─── Localization texts (CSV) ───

def diff_localization_texts(old_dir: Path, new_dir: Path) -> list[dict]:
    """Find untranslated entries in new localizationtexts/ CSV files."""
    new_loc = new_dir / "ExportedProject" / "Assets" / "StreamingAssets" / "TranslationPatch" / "localizationtexts"
    if not new_loc.exists():
        return []

    items = []
    for f in sorted(new_loc.glob("*.txt")):
        lines = f.read_text(encoding="utf-8-sig").splitlines()
        for line in lines[1:]:  # skip CSV header
            parts = line.split(",", 2)
            if len(parts) < 2:
                continue
            csv_id = parts[0].strip()
            en = parts[1].strip().strip('"')
            ko = parts[2].strip().strip('"') if len(parts) > 2 else ""

            if not en or not is_translatable(en):
                continue
            if ko:  # already translated
                continue

            h = hashlib.md5((f.stem + ":" + csv_id).encode("utf-8")).hexdigest()[:12]
            items.append({
                "id": f"loc-upd-{f.stem}-{h}",
                "source": en,
                "text_role": "ui_label",
                "source_type": "loctext_update",
                "source_file": f.name,
            })
    return items


# ─── Main ───

def main():
    parser = argparse.ArgumentParser(description="Diff game extract versions for translation pipeline")
    parser.add_argument("--old", required=True, help="Old version directory name (e.g. 1.1.1)")
    parser.add_argument("--new", required=True, help="New version directory name (e.g. 1.1.3)")
    parser.add_argument("--output", default=None, help="Output JSON path (default: auto-generated)")
    parser.add_argument("--apply", action="store_true", help="Insert into pipeline")
    parser.add_argument("--dry-run", action="store_true", help="Show what would be done")
    args = parser.parse_args()

    old_dir = EXTRACT_DIR / args.old
    new_dir = EXTRACT_DIR / args.new
    if not old_dir.exists():
        print(f"Error: old extract dir not found: {old_dir}", file=sys.stderr)
        sys.exit(1)
    if not new_dir.exists():
        print(f"Error: new extract dir not found: {new_dir}", file=sys.stderr)
        sys.exit(1)

    print(f"Comparing {args.old} -> {args.new}\n")

    # 1. TextAsset diff
    print("1. TextAsset (ink dialogue)...")
    ta_items = diff_textassets(old_dir, new_dir)
    print(f"   New dialogue strings: {len(ta_items)}")

    # 2. Overlay diff
    print("2. Overlay (translations.json)...")
    ovl_items = diff_overlay(old_dir, new_dir)
    enriched = sum(1 for i in ovl_items if i.get("context_en"))
    print(f"   New overlay entries: {len(ovl_items)} (with context: {enriched})")

    # 3. Localization texts diff
    print("3. Localization texts (CSV)...")
    loc_items = diff_localization_texts(old_dir, new_dir)
    print(f"   New untranslated: {len(loc_items)}")

    all_items = ta_items + ovl_items + loc_items
    print(f"\nTotal: {len(all_items)} items")

    if not all_items:
        print("No new items.")
        return

    # Output JSON
    output_path = Path(args.output) if args.output else (
        BATCH_DIR / f"diff_{args.old}_to_{args.new}.json"
    )
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(json.dumps(all_items, ensure_ascii=False, indent=2), encoding="utf-8")
    print(f"Written to: {output_path}")

    # Ingest
    ingest = PipelineIngest(batch_dir=BATCH_DIR)
    ingest.add(all_items, filter_translatable=False)  # already filtered above

    if args.dry_run:
        result = ingest.dry_run()
        print(f"\n[dry-run]\n{result.summary()}")
    elif args.apply:
        result = ingest.apply(initial_state="pending_translate")
        print(f"\n{result.summary()}")
    else:
        print(f"\nTo apply: python {__file__} --old {args.old} --new {args.new} --apply")


if __name__ == "__main__":
    main()
