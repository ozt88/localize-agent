import argparse
import json
from collections import Counter, defaultdict
from pathlib import Path


def load_json(path):
    return json.loads(Path(path).read_text(encoding="utf-8"))


def dump_json(path, obj):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(obj, ensure_ascii=False, indent=2), encoding="utf-8")


def is_dialogue_unit(unit):
    if unit.get("work_profile") == "dialogue":
        return True
    kinds = set(unit.get("blueprint_context_summary", {}).get("blueprint_kinds", []))
    return bool(kinds & {"dialogue_line", "dialogue_answer", "bark"})


def informative_conversation_groups(unit):
    groups = []
    blueprint_summary = unit.get("blueprint_context_summary", {})
    kinds = set(blueprint_summary.get("blueprint_kinds", []))
    for group in blueprint_summary.get("conversation_groups", []):
        if not group:
            continue
        lowered = group.lower()
        if lowered.startswith("cue_") or lowered.startswith("answer_"):
            continue
        if kinds & {"dialogue_line", "dialogue_answer"} and group == unit.get("blueprint_context_summary", {}).get("blueprint_names", [""])[0].replace(".Text", ""):
            continue
        groups.append(group)
    return groups


def group_key_for_unit(unit):
    blueprint_summary = unit.get("blueprint_context_summary", {})
    scene_summary = unit.get("scene_context_summary", {})
    dialog_refs = blueprint_summary.get("dialog_refs", [])
    if len(dialog_refs) == 1:
        first = dialog_refs[0]
        return f"dialog::{first.get('name') or first.get('guid')}"
    conversation_groups = informative_conversation_groups(unit)
    if conversation_groups:
        return conversation_groups[0]
    scene_files = scene_summary.get("scene_files", [])
    if len(scene_files) == 1:
        return f"scene::{scene_files[0]}"
    speaker_hints = blueprint_summary.get("speaker_hints", [])
    if len(scene_files) > 1 and speaker_hints:
        return f"speaker::{speaker_hints[0]}"
    return f"unit::{unit['unit_id']}"


def sort_key_for_unit(unit):
    sequence_hints = unit.get("blueprint_context_summary", {}).get("sequence_hints", [])
    first_seq = sequence_hints[0] if sequence_hints else 10**9
    return (
        first_seq,
        unit.get("source_text", ""),
        unit["unit_id"],
    )


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--canonical-source-json", required=True)
    ap.add_argument("--out-json", required=True)
    ap.add_argument("--out-summary", required=True)
    args = ap.parse_args()

    canonical = load_json(args.canonical_source_json)
    dialogue_units = [unit for unit in canonical.get("units", []) if is_dialogue_unit(unit)]

    grouped = defaultdict(list)
    for unit in dialogue_units:
        grouped[group_key_for_unit(unit)].append(unit)

    groups = []
    profile_counts = Counter()
    kind_counts = Counter()

    for group_key, units in sorted(grouped.items()):
        ordered_units = sorted(units, key=sort_key_for_unit)
        scene_files = sorted({scene for unit in ordered_units for scene in unit.get("scene_context_summary", {}).get("scene_files", [])})
        speaker_hints = sorted({speaker for unit in ordered_units for speaker in unit.get("blueprint_context_summary", {}).get("speaker_hints", [])})
        blueprint_kinds = sorted({kind for unit in ordered_units for kind in unit.get("blueprint_context_summary", {}).get("blueprint_kinds", [])})
        for unit in ordered_units:
            profile_counts[unit.get("work_profile", "general")] += 1
            for kind in unit.get("blueprint_context_summary", {}).get("blueprint_kinds", []):
                kind_counts[kind] += 1
        groups.append(
            {
                "group_key": group_key,
                "unit_count": len(ordered_units),
                "scene_files": scene_files,
                "speaker_hints": speaker_hints,
                "blueprint_kinds": blueprint_kinds,
                "units": ordered_units,
            }
        )

    out = {
        "format": "rogue-trader-dialogue-translation-export.v1",
        "source_format": canonical.get("format", ""),
        "group_count": len(groups),
        "unit_count": len(dialogue_units),
        "groups": groups,
    }
    summary = {
        "group_count": len(groups),
        "unit_count": len(dialogue_units),
        "work_profile_counts": dict(sorted(profile_counts.items())),
        "blueprint_kind_counts": dict(sorted(kind_counts.items())),
        "top_groups": [
            {
                "group_key": group["group_key"],
                "unit_count": group["unit_count"],
                "scene_files": group["scene_files"],
                "speaker_hints": group["speaker_hints"],
                "blueprint_kinds": group["blueprint_kinds"],
            }
            for group in sorted(groups, key=lambda item: (-item["unit_count"], item["group_key"]))[:20]
        ],
    }

    dump_json(args.out_json, out)
    dump_json(args.out_summary, summary)
    print(json.dumps(summary, ensure_ascii=False))


if __name__ == "__main__":
    main()
