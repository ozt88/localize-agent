import argparse
import json
import re
from collections import defaultdict
from pathlib import Path

import UnityPy


GUID_RE = re.compile(r"^[0-9a-f]{32}$", re.IGNORECASE)
UUID_RE = re.compile(r"^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$", re.IGNORECASE)
SPEAKER_CANDIDATE_RE = re.compile(
    r"(Abelard|Cassia|Heinrix|Argenta|Pasqal|Idira|Jae|Yrliet|Marazhai|Ulfar|Kibellah|Nomos|Theodora|Calcazar)",
    re.IGNORECASE,
)
CUE_RE = re.compile(r"(^|_)(Cue)[_\-]?0*(\d+)(_|\.|$)", re.IGNORECASE)
ANSWER_RE = re.compile(r"(^|_)(Answer)[_\-]?0*(\d+)(_|\.|$)", re.IGNORECASE)
TEXT_SUFFIX_RE = re.compile(r"\.(Text|Description|DisplayName|LocalizedName|LocalizedDescription)$", re.IGNORECASE)


def dump_json(path, obj):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(obj, ensure_ascii=False, indent=2), encoding="utf-8")


def load_cheatdata(path):
    payload = json.loads(Path(path).read_text(encoding="utf-8"))
    return {entry["Guid"].lower(): entry for entry in payload.get("Entries", []) if entry.get("Guid")}


def load_optional_json(path):
    if not path:
        return {}
    target = Path(path)
    if not target.exists():
        return {}
    return json.loads(target.read_text(encoding="utf-8"))


def normalize_guid(text):
    if not isinstance(text, str):
        return ""
    lowered = text.strip().lower()
    if GUID_RE.match(lowered):
        return lowered
    return ""


def normalize_bbp_guid(guid32):
    guid32 = (guid32 or "").strip().lower()
    if len(guid32) != 32:
        return ""
    return f"{guid32[0:8]}-{guid32[8:12]}-{guid32[12:16]}-{guid32[16:20]}-{guid32[20:32]}"


def normalize_name(name):
    base = (name or "").strip()
    if base.endswith("_.Text"):
        base = base[:-6]
    elif base.endswith(".Text"):
        base = base[:-5]
    return base.rstrip("_.")


def infer_speaker_hint(*values):
    for value in values:
        if not isinstance(value, str):
            continue
        match = SPEAKER_CANDIDATE_RE.search(value)
        if match:
            return match.group(1)
    return ""


def parse_blueprint_name_metadata(blueprint_name):
    speaker_hint = infer_speaker_hint(blueprint_name)
    normalized = blueprint_name.strip()
    base = TEXT_SUFFIX_RE.sub("", normalized)
    metadata = {
        "conversation_group": "",
        "sequence_hint": None,
        "name_kind_hint": "",
        "speaker_hint": speaker_hint,
    }

    cue_match = CUE_RE.search(base)
    if cue_match:
        metadata["sequence_hint"] = int(cue_match.group(3))
        metadata["name_kind_hint"] = "cue"
        metadata["conversation_group"] = base[: cue_match.start()].rstrip("_.-")
        return metadata

    answer_match = ANSWER_RE.search(base)
    if answer_match:
        metadata["sequence_hint"] = int(answer_match.group(3))
        metadata["name_kind_hint"] = "answer"
        metadata["conversation_group"] = base[: answer_match.start()].rstrip("_.-")
        return metadata

    if "bark" in base.lower():
        metadata["name_kind_hint"] = "bark"
        bark_index = base.lower().find("bark")
        metadata["conversation_group"] = base[:bark_index].rstrip("_.-")
        return metadata

    if any(token in base.lower() for token in ("dialog", "dialogue")):
        metadata["name_kind_hint"] = "dialog"
        metadata["conversation_group"] = base
        return metadata

    metadata["conversation_group"] = base
    return metadata


def is_generic_dialogue_node(name):
    lowered = (name or "").strip().lower()
    return bool(CUE_RE.search(lowered) or ANSWER_RE.search(lowered))


def infer_text_role(field_path, blueprint_name):
    tail = field_path.split(".")[-1]
    blueprint_lower = blueprint_name.lower()
    if tail == "String":
        if blueprint_name.endswith(".Text") or blueprint_lower.endswith("_text"):
            return "dialogue_text"
        if blueprint_name.endswith(".Description"):
            return "description"
        if blueprint_name.endswith(".DisplayName") or blueprint_name.endswith(".LocalizedName"):
            return "name_or_title"
        if "bark" in blueprint_lower:
            return "bark"
        return "string_value"
    if tail in {"Text", "m_Text"} or blueprint_lower.endswith("_cue") or "_cue" in blueprint_lower:
        return "dialogue_text"
    if tail in {"Description", "m_Description", "LocalizedDescription"}:
        return "description"
    if tail in {"DisplayName", "LocalizedName", "Title", "Name"}:
        return "name_or_title"
    if tail in {"m_TooltipDescription", "TooltipDescription"}:
        return "tooltip"
    if "bark" in blueprint_lower:
        return "bark"
    if tail == "AdditionalString":
        return "additional_string"
    return tail


def infer_blueprint_kind(blueprint_name, guid_refs):
    lowered = blueprint_name.lower()
    metadata = parse_blueprint_name_metadata(blueprint_name)
    if metadata["name_kind_hint"] == "cue":
        return "dialogue_line"
    if metadata["name_kind_hint"] == "answer":
        return "dialogue_answer"
    if metadata["name_kind_hint"] == "bark":
        return "bark"
    if ".text" in lowered or lowered.startswith("answer_") or lowered.startswith("cue_"):
        return "dialogue_line"
    if "answer" in lowered:
        return "dialogue_answer"
    if "cue" in lowered or "dialog" in lowered:
        return "dialogue"
    if "bark" in lowered:
        return "bark"
    if "quest" in lowered or "objective" in lowered or "contract" in lowered:
        return "quest"
    if "encyclopedia" in lowered or "glossary" in lowered or "lore" in lowered:
        return "encyclopedia"
    if "uistr" in lowered or lowered.startswith("uistrings") or "ui" in lowered:
        return "ui"
    if "_item_" in lowered or lowered.endswith("_item") or "item" in lowered:
        return "item"
    if "_feature_" in lowered or "feature" in lowered or "_ability_" in lowered or "ability" in lowered:
        return "ability_or_feature"
    if guid_refs:
        for ref in guid_refs:
            type_name = ref.get("type_full_name", "").lower()
            if "blueprintdialog" in type_name:
                return "dialogue"
            if "quest" in type_name:
                return "quest"
    return "generic_blueprint"


def scan_tree(node, path="", key_hits=None, guid_hits=None):
    if key_hits is None:
        key_hits = []
    if guid_hits is None:
        guid_hits = []

    if isinstance(node, dict):
        if "m_Key" in node and isinstance(node["m_Key"], str) and UUID_RE.match(node["m_Key"]):
            key_hits.append({"field_path": path or "m_Key", "key": node["m_Key"].lower()})
        if "guid" in node:
            guid = normalize_guid(node.get("guid"))
            if guid:
                guid_hits.append({"field_path": f"{path}.guid" if path else "guid", "guid": guid})
        for child_key, child_value in node.items():
            child_path = f"{path}.{child_key}" if path else child_key
            scan_tree(child_value, child_path, key_hits, guid_hits)
    elif isinstance(node, list):
        for index, item in enumerate(node):
            child_path = f"{path}[{index}]"
            scan_tree(item, child_path, key_hits, guid_hits)
    return key_hits, guid_hits


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--bundle-file", required=True)
    ap.add_argument("--cheatdata-json", required=True)
    ap.add_argument("--bbp-links-json", default="")
    ap.add_argument("--out-json", required=True)
    args = ap.parse_args()

    cheat_idx = load_cheatdata(args.cheatdata_json)
    bbp_links = load_optional_json(args.bbp_links_json)
    bbp_link_index = bbp_links.get("node_link_index", {}) if isinstance(bbp_links, dict) else {}
    env = UnityPy.load(args.bundle_file)

    string_key_index = defaultdict(list)
    guid_index = defaultdict(list)
    object_count = 0
    object_with_key_count = 0
    guid_ref_count = 0

    for obj in env.objects:
        if obj.type.name != "MonoBehaviour":
            continue
        object_count += 1
        try:
            tree = obj.read_typetree()
        except Exception:
            continue
        blueprint_name = tree.get("m_Name", "")
        key_hits, guid_hits = scan_tree(tree)
        if key_hits:
            object_with_key_count += 1
        name_metadata = parse_blueprint_name_metadata(blueprint_name)
        speaker_hint = name_metadata["speaker_hint"]
        resolved_guids = []
        for item in guid_hits:
            meta = cheat_idx.get(item["guid"], {})
            resolved = {
                "guid": item["guid"],
                "field_path": item["field_path"],
                "name": meta.get("Name", ""),
                "type_full_name": meta.get("TypeFullName", ""),
            }
            resolved_guids.append(resolved)
            guid_index[item["guid"]].append(
                {
                    "bundle_file": Path(args.bundle_file).name,
                    "component_path_id": obj.path_id,
                    "blueprint_name": blueprint_name,
                    "speaker_hint": speaker_hint,
                    "guid_ref": resolved,
                }
            )
            guid_ref_count += 1

        for key_hit in key_hits:
            blueprint_kind = infer_blueprint_kind(blueprint_name, resolved_guids)
            normalized_name = normalize_name(blueprint_name)
            bbp_links_for_name = bbp_link_index.get(normalized_name, [])
            bbp_dialog_names = sorted({item.get("dialog_name", "") for item in bbp_links_for_name if item.get("dialog_name")})
            bbp_dialog_refs = [
                {
                    "dialog_name": item.get("dialog_name", ""),
                    "dialog_guid": normalize_bbp_guid(item.get("dialog_guid32", "")),
                    "node_name": item.get("node_name", ""),
                }
                for item in bbp_links_for_name
            ]
            conversation_group = name_metadata["conversation_group"]
            if not conversation_group and bbp_dialog_refs and (not is_generic_dialogue_node(normalized_name) or len(bbp_dialog_names) <= 3):
                conversation_group = bbp_dialog_refs[0]["dialog_name"]
            string_key_index[key_hit["key"]].append(
                {
                    "bundle_file": Path(args.bundle_file).name,
                    "component_path_id": obj.path_id,
                    "blueprint_name": blueprint_name,
                    "blueprint_kind": blueprint_kind,
                    "conversation_group": conversation_group,
                    "sequence_hint": name_metadata["sequence_hint"],
                    "name_kind_hint": name_metadata["name_kind_hint"],
                    "speaker_hint": speaker_hint,
                    "field_path": key_hit["field_path"],
                    "text_role": infer_text_role(key_hit["field_path"], blueprint_name),
                    "bbp_dialog_refs": bbp_dialog_refs,
                    "guid_refs": resolved_guids,
                }
            )

    out = {
        "format": "rogue-trader-blueprint-context.v1",
        "bundle_file": str(Path(args.bundle_file)),
        "object_count": object_count,
        "object_with_key_count": object_with_key_count,
        "string_key_count": len(string_key_index),
        "guid_count": len(guid_index),
        "guid_ref_count": guid_ref_count,
        "string_key_index": dict(sorted(string_key_index.items())),
        "guid_index": dict(sorted(guid_index.items())),
    }
    dump_json(args.out_json, out)
    print(
        json.dumps(
            {
                "object_count": object_count,
                "object_with_key_count": object_with_key_count,
                "string_key_count": len(string_key_index),
                "guid_count": len(guid_index),
                "guid_ref_count": guid_ref_count,
            },
            ensure_ascii=False,
        )
    )


if __name__ == "__main__":
    main()
