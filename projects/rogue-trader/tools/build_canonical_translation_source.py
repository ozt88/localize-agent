import argparse
import hashlib
import json
import re
from collections import Counter, defaultdict
from datetime import datetime, timezone
from pathlib import Path


GLOSSARY_TAG_RE = re.compile(r"\{g\|([^}]+)\}(.+?)\{/g\}", re.IGNORECASE | re.DOTALL)
HTML_TAG_RE = re.compile(r"</?[^>]+>")
MF_TAG_RE = re.compile(r"\{mf\|[^}]+\}", re.IGNORECASE)
BRACE_TOKEN_RE = re.compile(r"\{[^{}]+\}")
SQUARE_TOKEN_RE = re.compile(r"\[[^\[\]]+\]")
PRINTF_TOKEN_RE = re.compile(r"%(?:\d+\$)?[+#0\- ]*(?:\d+)?(?:\.\d+)?[a-zA-Z]")
ANGLE_VAR_RE = re.compile(r"<[^>]+>")
WHITESPACE_RE = re.compile(r"\s+")
LETTER_RE = re.compile(r"[A-Za-z]")
SPEAKER_CANDIDATE_RE = re.compile(
    r"(Abelard|Cassia|Heinrix|Argenta|Pasqal|Idira|Jae|Yrliet|Marazhai|Ulfar|Kibellah|Nomos|Theodora|Calcazar)",
    re.IGNORECASE,
)


def load_json(path):
    return json.loads(Path(path).read_text(encoding="utf-8"))


def dump_json(path, obj):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(obj, ensure_ascii=False, indent=2), encoding="utf-8")


def load_optional_json(path):
    if not path:
        return {}
    target = Path(path)
    if not target.exists():
        return {}
    return json.loads(target.read_text(encoding="utf-8"))


def stable_hash(text):
    return hashlib.sha1(text.encode("utf-8")).hexdigest()[:12]


def normalize_whitespace(text):
    return WHITESPACE_RE.sub(" ", text).strip()


def split_words(text):
    return [part for part in re.split(r"\s+", text.strip()) if part]


def extract_glossary_terms(payload):
    terms = {}

    def add_term(en_term, ko_value, category, aliases=None):
        if not isinstance(en_term, str):
            return
        en_term = en_term.strip()
        if not en_term:
            return
        entry = terms.setdefault(
            en_term.lower(),
            {
                "term": en_term,
                "ko": ko_value if isinstance(ko_value, str) else "",
                "category": category,
                "aliases": set(),
            },
        )
        if isinstance(aliases, list):
            for alias in aliases:
                if isinstance(alias, str) and alias.strip():
                    entry["aliases"].add(alias.strip())

    for section, value in payload.items():
        if section == "Lore_Termbank" and isinstance(value, dict):
            for en_term, meta in value.items():
                if isinstance(meta, dict):
                    add_term(en_term, meta.get("ko", ""), meta.get("category", section), meta.get("aliases", []))
                else:
                    add_term(en_term, "", section, [])
        elif section == "Skill_Identifiers" and isinstance(value, dict):
            entries = value.get("entries", {})
            if isinstance(entries, dict):
                for en_term, ko_value in entries.items():
                    add_term(en_term, ko_value, section, [])
        elif isinstance(value, dict):
            for en_term, ko_value in value.items():
                if isinstance(ko_value, str):
                    add_term(en_term, ko_value, section, [])

    results = []
    for item in terms.values():
        aliases = sorted(item["aliases"])
        results.append(
            {
                "term": item["term"],
                "ko": item["ko"],
                "category": item["category"],
                "aliases": aliases,
            }
        )
        for alias in aliases:
            results.append(
                {
                    "term": alias,
                    "ko": item["ko"],
                    "category": item["category"],
                    "aliases": [],
                }
            )
    results.sort(key=lambda item: (-len(item["term"]), item["term"].lower()))
    return results


def infer_speaker_hint(*values):
    for value in values:
        if not isinstance(value, str):
            continue
        match = SPEAKER_CANDIDATE_RE.search(value)
        if match:
            return match.group(1)
    return ""


def collect_matches(pattern, text):
    return sorted({match.group(0) for match in pattern.finditer(text)})


def extract_preservation(text):
    glossary_tags = []
    glossary_labels = []
    glossary_keys = []
    for match in GLOSSARY_TAG_RE.finditer(text):
        glossary_tags.append(match.group(0))
        glossary_keys.append(match.group(1))
        glossary_labels.append(match.group(2))
    return {
        "glossary_tags": glossary_tags,
        "glossary_keys": sorted(set(glossary_keys)),
        "glossary_labels": sorted(set(glossary_labels)),
        "html_tags": collect_matches(HTML_TAG_RE, text),
        "mf_tags": collect_matches(MF_TAG_RE, text),
        "brace_tokens": collect_matches(BRACE_TOKEN_RE, text),
        "square_tokens": collect_matches(SQUARE_TOKEN_RE, text),
        "printf_tokens": collect_matches(PRINTF_TOKEN_RE, text),
        "angle_tokens": collect_matches(ANGLE_VAR_RE, text),
    }


def classify_text(text, preservation):
    stripped = text.strip()
    char_count = len(stripped)
    word_count = len(split_words(stripped))
    sentence_breaks = stripped.count(".") + stripped.count("!") + stripped.count("?")
    has_markup = bool(
        preservation["glossary_tags"]
        or preservation["html_tags"]
        or preservation["mf_tags"]
        or preservation["brace_tokens"]
        or preservation["square_tokens"]
        or preservation["printf_tokens"]
    )
    if "\n" in stripped:
        return "multi_line_block"
    if has_markup and char_count <= 72:
        return "templated_label"
    if has_markup:
        return "templated_description"
    if char_count <= 3 and LETTER_RE.search(stripped):
        return "single_token"
    if word_count <= 4 and sentence_breaks == 0 and not stripped.endswith("."):
        if stripped.endswith(":"):
            return "ui_label"
        if stripped.istitle() or stripped.isupper():
            return "title_or_name"
        return "short_label"
    if sentence_breaks >= 2 or char_count >= 220:
        return "long_description"
    return "sentence"


def build_glossary_hits(text, glossary_terms, max_hits):
    lowered = text.lower()
    hits = []
    seen = set()
    for item in glossary_terms:
        needle = item["term"].lower()
        if len(needle) < 3:
            continue
        if needle in lowered:
            key = (item["category"], item["term"].lower())
            if key in seen:
                continue
            seen.add(key)
            hits.append(
                {
                    "term": item["term"],
                    "ko": item["ko"],
                    "category": item["category"],
                }
            )
            if len(hits) >= max_hits:
                break
    return hits


def summarize_top_duplicates(groups, limit):
    preview = []
    ranked = [
        items
        for items in sorted(groups.values(), key=lambda items: (-len(items), items[0]["text"]))
        if items and items[0]["text"].strip()
    ]
    for items in ranked[:limit]:
        preview.append(
            {
                "source_text": items[0]["text"],
                "group_size": len(items),
                "sample_ids": [item["id"] for item in items[:5]],
            }
        )
    return preview


def summarize_scene_contexts(scene_contexts):
    if not scene_contexts:
        return {
            "count": 0,
            "scene_files": [],
            "script_names": [],
            "speaker_hints": [],
            "dialog_refs": [],
        }
    scene_files = sorted({item.get("scene_file", "") for item in scene_contexts if item.get("scene_file")})
    script_names = sorted({item.get("script_name", "") for item in scene_contexts if item.get("script_name")})
    speaker_hints = sorted({item.get("speaker_hint", "") for item in scene_contexts if item.get("speaker_hint")})
    dialog_refs = []
    seen = set()
    for item in scene_contexts:
        for ref in item.get("guid_refs", []):
            guid = ref.get("guid", "")
            if not guid or guid in seen:
                continue
            seen.add(guid)
            dialog_refs.append(
                {
                    "guid": guid,
                    "name": ref.get("name", ""),
                    "type_full_name": ref.get("type_full_name", ""),
                }
            )
    dialog_refs.sort(key=lambda item: (item["name"], item["guid"]))
    return {
        "count": len(scene_contexts),
        "scene_files": scene_files,
        "script_names": script_names,
        "speaker_hints": speaker_hints,
        "dialog_refs": dialog_refs,
    }


def summarize_blueprint_contexts(blueprint_contexts):
    if not blueprint_contexts:
        return {
            "count": 0,
            "blueprint_names": [],
            "blueprint_kinds": [],
            "conversation_groups": [],
            "sequence_hints": [],
            "text_roles": [],
            "speaker_hints": [],
            "dialog_refs": [],
        }
    blueprint_names = sorted({item.get("blueprint_name", "") for item in blueprint_contexts if item.get("blueprint_name")})
    blueprint_kinds = sorted({item.get("blueprint_kind", "") for item in blueprint_contexts if item.get("blueprint_kind")})
    conversation_groups = sorted({item.get("conversation_group", "") for item in blueprint_contexts if item.get("conversation_group")})
    sequence_hints = sorted({item.get("sequence_hint") for item in blueprint_contexts if item.get("sequence_hint") is not None})
    text_roles = sorted({item.get("text_role", "") for item in blueprint_contexts if item.get("text_role")})
    speaker_hints = sorted({item.get("speaker_hint", "") for item in blueprint_contexts if item.get("speaker_hint")})
    dialog_refs = []
    seen = set()
    for item in blueprint_contexts:
        for ref in item.get("guid_refs", []):
            guid = ref.get("guid", "")
            if not guid or guid in seen:
                continue
            seen.add(guid)
            dialog_refs.append(
                {
                    "guid": guid,
                    "name": ref.get("name", ""),
                    "type_full_name": ref.get("type_full_name", ""),
                }
            )
        for ref in item.get("bbp_dialog_refs", []):
            guid = ref.get("dialog_guid", "") or ref.get("dialog_name", "")
            if not guid or guid in seen:
                continue
            seen.add(guid)
            dialog_refs.append(
                {
                    "guid": ref.get("dialog_guid", ""),
                    "name": ref.get("dialog_name", ""),
                    "type_full_name": "bbp_dialogue_link",
                }
            )
    dialog_refs.sort(key=lambda item: (item["name"], item["guid"]))
    return {
        "count": len(blueprint_contexts),
        "blueprint_names": blueprint_names,
        "blueprint_kinds": blueprint_kinds,
        "conversation_groups": conversation_groups,
        "sequence_hints": sequence_hints,
        "text_roles": text_roles,
        "speaker_hints": speaker_hints,
        "dialog_refs": dialog_refs,
    }


def summarize_sound_context(sound_contexts):
    if not sound_contexts:
        return {
            "count": 0,
            "cue_names": [],
            "speaker_hints": [],
            "voice_kinds": [],
        }
    cue_names = sorted({item.get("cue_name", "") for item in sound_contexts if item.get("cue_name")})
    speaker_hints = sorted({item.get("speaker_hint", "") for item in sound_contexts if item.get("speaker_hint")})
    voice_kinds = sorted({item.get("voice_kind", "") for item in sound_contexts if item.get("voice_kind")})
    return {
        "count": len(sound_contexts),
        "cue_names": cue_names,
        "speaker_hints": speaker_hints,
        "voice_kinds": voice_kinds,
    }


def build_sound_context_map(sound_payload):
    strings = sound_payload.get("strings", {}) if isinstance(sound_payload, dict) else {}
    out = {}
    for key, value in strings.items():
        cue_name = value.get("Text", "") if isinstance(value, dict) else ""
        if not cue_name:
            continue
        upper = cue_name.upper()
        if upper.startswith("BNTRS_"):
            voice_kind = "banter"
        elif upper.startswith("NARR_"):
            voice_kind = "narration"
        elif upper.startswith("DLOG_"):
            voice_kind = "dialogue"
        else:
            voice_kind = "voice_or_audio"
        out[key] = [
            {
                "cue_name": cue_name,
                "voice_kind": voice_kind,
                "speaker_hint": infer_speaker_hint(cue_name),
            }
        ]
    return out


def infer_work_profile(blueprint_summary, scene_summary, text_kind):
    blueprint_kinds = set(blueprint_summary.get("blueprint_kinds", []))
    text_roles = set(blueprint_summary.get("text_roles", []))
    if "dialogue_line" in blueprint_kinds or "dialogue_answer" in blueprint_kinds or "bark" in blueprint_kinds:
        return "dialogue"
    if "quest" in blueprint_kinds:
        return "quest"
    if "encyclopedia" in blueprint_kinds:
        return "encyclopedia"
    if "item" in blueprint_kinds or "ability_or_feature" in blueprint_kinds:
        return "system_content"
    if scene_summary.get("count", 0) > 0:
        return "scene_bound"
    if text_kind in {"ui_label", "templated_label", "short_label", "title_or_name"}:
        return "ui_or_name"
    if "description" in text_roles or text_kind in {"long_description", "templated_description"}:
        return "descriptive"
    return "general"


def build_scene_batches(units):
    batches = {}
    for unit in units:
        scene_files = unit.get("scene_context_summary", {}).get("scene_files", [])
        if not scene_files:
            continue
        for scene_file in scene_files:
            batch = batches.setdefault(
                scene_file,
                {
                    "scene_file": scene_file,
                    "unit_ids": [],
                    "unit_count": 0,
                    "profiles": Counter(),
                    "speaker_hints": Counter(),
                    "blueprint_kinds": Counter(),
                    "conversation_groups": Counter(),
                },
            )
            batch["unit_ids"].append(unit["unit_id"])
            batch["unit_count"] += 1
            batch["profiles"][unit.get("work_profile", "general")] += 1
            for speaker in unit.get("blueprint_context_summary", {}).get("speaker_hints", []):
                batch["speaker_hints"][speaker] += 1
            for kind in unit.get("blueprint_context_summary", {}).get("blueprint_kinds", []):
                batch["blueprint_kinds"][kind] += 1
            for group in unit.get("blueprint_context_summary", {}).get("conversation_groups", []):
                batch["conversation_groups"][group] += 1
    out = []
    for scene_file, batch in sorted(batches.items()):
        out.append(
            {
                "scene_file": scene_file,
                "unit_count": batch["unit_count"],
                "unit_ids": batch["unit_ids"],
                "profiles": dict(batch["profiles"].most_common()),
                "speaker_hints": [speaker for speaker, _ in batch["speaker_hints"].most_common(10)],
                "blueprint_kinds": dict(batch["blueprint_kinds"].most_common()),
                "conversation_groups": [group for group, _ in batch["conversation_groups"].most_common(20)],
            }
        )
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--source-json", required=True)
    ap.add_argument("--current-json", required=True)
    ap.add_argument("--glossary-json", default="workflow/context/universal_glossary.json")
    ap.add_argument("--extract-root", default="")
    ap.add_argument("--scene-context-json", default="")
    ap.add_argument("--blueprint-context-json", default="")
    ap.add_argument("--sound-json", default="")
    ap.add_argument("--out-json", required=True)
    ap.add_argument("--out-reference-json", required=True)
    ap.add_argument("--out-summary", required=True)
    ap.add_argument("--out-scene-batches-json", default="")
    ap.add_argument("--max-glossary-hits", type=int, default=12)
    args = ap.parse_args()

    source = load_json(args.source_json)
    current = load_json(args.current_json)
    glossary = load_json(args.glossary_json) if args.glossary_json else {}
    scene_context = load_optional_json(args.scene_context_json)
    blueprint_context = load_optional_json(args.blueprint_context_json)
    sound_payload = load_optional_json(args.sound_json)
    glossary_terms = extract_glossary_terms(glossary)
    scene_context_by_key = scene_context.get("string_key_index", {}) if isinstance(scene_context, dict) else {}
    blueprint_context_by_key = blueprint_context.get("string_key_index", {}) if isinstance(blueprint_context, dict) else {}
    sound_context_by_key = build_sound_context_map(sound_payload)

    source_strings = source.get("strings", {})
    current_strings = current.get("strings", {})

    groups = defaultdict(list)
    total_rows = 0
    rows_with_current_ko = 0

    for string_id, source_obj in source_strings.items():
        text = source_obj.get("Text", "")
        current_obj = current_strings.get(string_id, {})
        current_ko = current_obj.get("Text", "") if isinstance(current_obj, dict) else ""
        if current_ko:
            rows_with_current_ko += 1
        groups[text].append(
            {
                "id": string_id,
                "text": text,
                "offset": source_obj.get("Offset"),
                "current_ko": current_ko,
            }
        )
        total_rows += 1

    units = []
    reference_units = []
    text_kind_counts = Counter()
    units_with_markup = 0
    units_with_glossary_hits = 0
    units_with_mixed_current_ko = 0
    units_with_empty_current_ko = 0
    empty_source_units = 0
    skip_units = 0

    for index, (source_text, items) in enumerate(sorted(groups.items(), key=lambda pair: (-len(pair[1]), pair[0])), start=1):
        preservation = extract_preservation(source_text)
        text_kind = classify_text(source_text, preservation)
        glossary_hits = build_glossary_hits(source_text, glossary_terms, args.max_glossary_hits)
        current_variants = sorted({item["current_ko"] for item in items if item["current_ko"]})
        scene_contexts = []
        seen_scene_contexts = set()
        for item in items:
            for context in scene_context_by_key.get(item["id"], []):
                fingerprint = json.dumps(context, ensure_ascii=False, sort_keys=True)
                if fingerprint in seen_scene_contexts:
                    continue
                seen_scene_contexts.add(fingerprint)
                scene_contexts.append(context)
        scene_context_summary = summarize_scene_contexts(scene_contexts)
        blueprint_contexts = []
        seen_blueprint_contexts = set()
        for item in items:
            for context in blueprint_context_by_key.get(item["id"], []):
                fingerprint = json.dumps(context, ensure_ascii=False, sort_keys=True)
                if fingerprint in seen_blueprint_contexts:
                    continue
                seen_blueprint_contexts.add(fingerprint)
                blueprint_contexts.append(context)
        blueprint_context_summary = summarize_blueprint_contexts(blueprint_contexts)
        sound_contexts = []
        for item in items:
            sound_contexts.extend(sound_context_by_key.get(item["id"], []))
        sound_context_summary = summarize_sound_context(sound_contexts)
        work_profile = infer_work_profile(blueprint_context_summary, scene_context_summary, text_kind)
        if not current_variants:
            current_status = "empty"
            units_with_empty_current_ko += 1
        elif len(current_variants) == 1:
            current_status = "consistent"
        else:
            current_status = "mixed"
            units_with_mixed_current_ko += 1

        translation_action = "translate"
        if source_text == "":
            translation_action = "skip_empty"
            empty_source_units += 1
            skip_units += 1

        unit_id = f"rt-canon-{index:05d}-{stable_hash(source_text)}"
        unit = {
            "unit_id": unit_id,
            "unit_kind": "exact_text_group",
            "source_text": source_text,
            "normalized_text": normalize_whitespace(source_text),
            "text_kind": text_kind,
            "translation_action": translation_action,
            "work_profile": work_profile,
            "group_size": len(items),
            "char_count": len(source_text),
            "word_count": len(split_words(source_text)),
            "line_count": source_text.count("\n") + 1,
            "current_ko_status": current_status,
            "current_ko_variants": current_variants,
            "glossary_hits": glossary_hits,
            "blueprint_context_summary": blueprint_context_summary,
            "blueprint_contexts": blueprint_contexts,
            "sound_context_summary": sound_context_summary,
            "sound_contexts": sound_contexts,
            "scene_context_summary": scene_context_summary,
            "scene_contexts": scene_contexts,
            "preservation": preservation,
            "lines": [
                {
                    "id": item["id"],
                    "body_en": item["text"],
                    "current_ko": item["current_ko"],
                    "offset": item["offset"],
                    "translation_lane": "shared_exact" if len(items) > 1 else "single_use",
                }
                for item in sorted(items, key=lambda row: row["id"])
            ],
        }
        reference_units.append(
            {
                "unit_id": unit_id,
                "group_size": len(items),
                "current_ko_status": current_status,
                "ids": [item["id"] for item in sorted(items, key=lambda row: row["id"])],
            }
        )
        units.append(unit)
        text_kind_counts[text_kind] += 1
        if any(preservation[key] for key in ("glossary_tags", "html_tags", "mf_tags", "brace_tokens", "square_tokens", "printf_tokens")):
            units_with_markup += 1
        if glossary_hits:
            units_with_glossary_hits += 1

    summary = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "source_rows": total_rows,
        "unique_units": len(units),
        "duplicate_rows_saved": total_rows - len(units),
        "multi_use_units": sum(1 for unit in units if unit["group_size"] > 1),
        "max_group_size": max((unit["group_size"] for unit in units), default=0),
        "rows_with_current_ko": rows_with_current_ko,
        "units_with_markup": units_with_markup,
        "units_with_glossary_hits": units_with_glossary_hits,
        "units_with_mixed_current_ko": units_with_mixed_current_ko,
        "units_with_empty_current_ko": units_with_empty_current_ko,
        "empty_source_units": empty_source_units,
        "skip_units": skip_units,
        "units_with_blueprint_context": sum(1 for unit in units if unit["blueprint_context_summary"]["count"] > 0),
        "units_with_sound_context": sum(1 for unit in units if unit["sound_context_summary"]["count"] > 0),
        "units_with_scene_context": sum(1 for unit in units if unit["scene_context_summary"]["count"] > 0),
        "work_profile_counts": dict(sorted(Counter(unit["work_profile"] for unit in units).items())),
        "text_kind_counts": dict(sorted(text_kind_counts.items())),
        "top_duplicate_units_preview": summarize_top_duplicates(groups, 10),
        "inputs": {
            "source_json": str(Path(args.source_json)),
            "current_json": str(Path(args.current_json)),
            "glossary_json": str(Path(args.glossary_json)) if args.glossary_json else "",
            "extract_root": str(Path(args.extract_root)) if args.extract_root else "",
            "scene_context_json": str(Path(args.scene_context_json)) if args.scene_context_json else "",
            "blueprint_context_json": str(Path(args.blueprint_context_json)) if args.blueprint_context_json else "",
            "sound_json": str(Path(args.sound_json)) if args.sound_json else "",
        },
    }

    out = {
        "format": "rogue-trader-canonical-translation-source.v1",
        "generated_at": summary["generated_at"],
        "instructions": {
            "translate_unit": "exact_text_group",
            "return_unit": "line",
            "rules": [
                "Translate one canonical unit once, then reuse it for every line in the group.",
                "Do not merge or split line ids.",
                "Preserve glossary tags, HTML tags, printf tokens, brace tokens, and square-bracket tokens exactly.",
                "Use current_ko_variants only as reference; mixed variants indicate a consistency problem to resolve canonically.",
                "Prefer glossary_hits when a lore or system term appears in source_text.",
            ],
        },
        "assets": {
            "extract_root": str(Path(args.extract_root)) if args.extract_root else "",
            "extract_context_status": "available" if args.extract_root else "not_provided",
            "scene_context_status": "available" if args.scene_context_json else "not_provided",
            "blueprint_context_status": "available" if args.blueprint_context_json else "not_provided",
            "sound_context_status": "available" if args.sound_json else "not_provided",
        },
        "summary": {
            "source_rows": summary["source_rows"],
            "unique_units": summary["unique_units"],
            "duplicate_rows_saved": summary["duplicate_rows_saved"],
            "multi_use_units": summary["multi_use_units"],
            "units_with_mixed_current_ko": summary["units_with_mixed_current_ko"],
            "units_with_blueprint_context": summary["units_with_blueprint_context"],
            "units_with_sound_context": summary["units_with_sound_context"],
            "units_with_scene_context": summary["units_with_scene_context"],
            "skip_units": summary["skip_units"],
        },
        "units": units,
    }

    reference = {
        "format": "rogue-trader-canonical-translation-reference.v1",
        "generated_at": summary["generated_at"],
        "units": reference_units,
    }

    scene_batches = build_scene_batches(units)

    dump_json(args.out_json, out)
    dump_json(args.out_reference_json, reference)
    dump_json(args.out_summary, summary)
    if args.out_scene_batches_json:
        dump_json(
            args.out_scene_batches_json,
            {
                "format": "rogue-trader-scene-batches.v1",
                "generated_at": summary["generated_at"],
                "scene_batches": scene_batches,
            },
        )
    print(json.dumps(summary, ensure_ascii=False))


if __name__ == "__main__":
    main()
