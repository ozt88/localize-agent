import argparse
import json
import re
from collections import Counter, defaultdict
from pathlib import Path

import UnityPy


GUID_RE = re.compile(r"^[0-9a-f]{32}$", re.IGNORECASE)
UUID_RE = re.compile(r"^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$", re.IGNORECASE)
SPEAKER_CANDIDATE_RE = re.compile(
    r"(Abelard|Cassia|Heinrix|Argenta|Pasqal|Idira|Jae|Yrliet|Marazhai|Ulfar|Kibellah|Nomos|Theodora|Calcazar)",
    re.IGNORECASE,
)


def dump_json(path, obj):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(obj, ensure_ascii=False, indent=2), encoding="utf-8")


def load_cheatdata(path):
    if not path:
        return {}
    payload = json.loads(Path(path).read_text(encoding="utf-8"))
    return {entry["Guid"].lower(): entry for entry in payload.get("Entries", []) if entry.get("Guid")}


def normalize_guid(text):
    if not isinstance(text, str):
        return ""
    lowered = text.strip().lower()
    if GUID_RE.match(lowered):
        return lowered
    return ""


def infer_speaker_hint(*values):
    for value in values:
        if not isinstance(value, str):
            continue
        match = SPEAKER_CANDIDATE_RE.search(value)
        if match:
            return match.group(1)
    return ""


def get_game_object_name(obj_by_path, pointer):
    if not isinstance(pointer, dict):
        return ""
    path_id = pointer.get("m_PathID")
    if not path_id:
        return ""
    target = obj_by_path.get(path_id)
    if target is None:
        return ""
    try:
        data = target.read()
        return getattr(data, "m_Name", "") or getattr(data, "name", "")
    except Exception:
        return ""


def get_script_name(data):
    script = getattr(data, "m_Script", None)
    if not script:
        return ""
    try:
        script_obj = script.read()
        return getattr(script_obj, "m_Name", "") or getattr(script_obj, "name", "")
    except Exception:
        return ""


def scan_tree(node, path="", key_hits=None, guid_hits=None, pptr_hits=None):
    if key_hits is None:
        key_hits = []
    if guid_hits is None:
        guid_hits = []
    if pptr_hits is None:
        pptr_hits = []

    if isinstance(node, dict):
        if "m_Key" in node and isinstance(node["m_Key"], str) and UUID_RE.match(node["m_Key"]):
            key_hits.append({"field_path": path or "m_Key", "key": node["m_Key"].lower()})
        if "guid" in node:
            guid = normalize_guid(node.get("guid"))
            if guid:
                guid_hits.append({"field_path": f"{path}.guid" if path else "guid", "guid": guid})
        if "m_PathID" in node and "m_FileID" in node:
            pptr_hits.append(
                {
                    "field_path": path,
                    "m_FileID": node.get("m_FileID"),
                    "m_PathID": node.get("m_PathID"),
                }
            )
        for child_key, child_value in node.items():
            child_path = f"{path}.{child_key}" if path else child_key
            scan_tree(child_value, child_path, key_hits, guid_hits, pptr_hits)
    elif isinstance(node, list):
        for index, item in enumerate(node):
            child_path = f"{path}[{index}]"
            scan_tree(item, child_path, key_hits, guid_hits, pptr_hits)

    return key_hits, guid_hits, pptr_hits


def resolve_pptr_string_keys(obj_by_path, pptr_hits):
    resolved = []
    for item in pptr_hits:
        path_id = item.get("m_PathID")
        if not path_id:
            continue
        target = obj_by_path.get(path_id)
        if target is None or target.type.name != "MonoBehaviour":
            continue
        try:
            target_data = target.read()
            script_name = get_script_name(target_data)
            if script_name != "SharedStringAsset":
                continue
            target_tree = target.read_typetree()
        except Exception:
            continue
        string_node = target_tree.get("String", {})
        key = string_node.get("m_Key", "")
        if isinstance(key, str) and UUID_RE.match(key):
            resolved.append(
                {
                    "field_path": item["field_path"],
                    "key": key.lower(),
                    "target_script": script_name,
                    "target_name": target_tree.get("m_Name", ""),
                    "target_path_id": path_id,
                }
            )
    return resolved


def build_guid_ref(guid, cheat_idx):
    ref = {"guid": guid}
    meta = cheat_idx.get(guid)
    if meta:
        ref["name"] = meta.get("Name", "")
        ref["type_full_name"] = meta.get("TypeFullName", "")
    return ref


def parse_scene(scene_path, cheat_idx):
    env = UnityPy.load(str(scene_path))
    obj_by_path = {obj.path_id: obj for obj in env.objects}
    component_records = []
    key_contexts = defaultdict(list)
    guid_contexts = defaultdict(list)
    script_counts = Counter()

    for obj in env.objects:
        if obj.type.name != "MonoBehaviour":
            continue
        try:
            data = obj.read()
            script_name = get_script_name(data)
            tree = obj.read_typetree()
        except Exception:
            continue

        script_counts[script_name or "<unknown>"] += 1
        game_object_name = get_game_object_name(obj_by_path, tree.get("m_GameObject"))
        key_hits, guid_hits, pptr_hits = scan_tree(tree)
        key_hits.extend(resolve_pptr_string_keys(obj_by_path, pptr_hits))

        if not key_hits and not guid_hits:
            continue

        resolved_guids = [build_guid_ref(item["guid"], cheat_idx) for item in guid_hits]
        speaker_hint = infer_speaker_hint(game_object_name, tree.get("m_Name", ""), *(ref.get("name", "") for ref in resolved_guids))
        record = {
            "scene_file": scene_path.name,
            "component_path_id": obj.path_id,
            "script_name": script_name,
            "component_name": tree.get("m_Name", ""),
            "game_object_name": game_object_name,
            "speaker_hint": speaker_hint,
            "string_keys": key_hits,
            "guid_refs": resolved_guids,
        }
        component_records.append(record)

        for key_hit in key_hits:
            key_contexts[key_hit["key"]].append(
                {
                    "scene_file": scene_path.name,
                    "script_name": script_name,
                    "component_name": tree.get("m_Name", ""),
                    "game_object_name": game_object_name,
                    "speaker_hint": speaker_hint,
                    "field_path": key_hit["field_path"],
                    "guid_refs": resolved_guids,
                }
            )
        for guid_ref in resolved_guids:
            guid_contexts[guid_ref["guid"]].append(
                {
                    "scene_file": scene_path.name,
                    "script_name": script_name,
                    "component_name": tree.get("m_Name", ""),
                    "game_object_name": game_object_name,
                    "speaker_hint": speaker_hint,
                    "guid_ref": guid_ref,
                }
            )

    summary = {
        "scene_file": scene_path.name,
        "components_with_context": len(component_records),
        "scripts": dict(script_counts.most_common(20)),
        "string_key_refs": sum(len(record["string_keys"]) for record in component_records),
        "guid_refs": sum(len(record["guid_refs"]) for record in component_records),
    }
    return component_records, key_contexts, guid_contexts, summary


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--bundles-root", required=True)
    ap.add_argument("--cheatdata-json", required=True)
    ap.add_argument("--glob", default="*mechanics.scenes")
    ap.add_argument("--limit", type=int, default=0)
    ap.add_argument("--out-json", required=True)
    args = ap.parse_args()

    bundles_root = Path(args.bundles_root)
    cheat_idx = load_cheatdata(args.cheatdata_json)
    scene_paths = sorted(bundles_root.glob(args.glob))
    if args.limit > 0:
        scene_paths = scene_paths[: args.limit]

    all_components = []
    key_index = defaultdict(list)
    guid_index = defaultdict(list)
    scene_summaries = []

    for scene_path in scene_paths:
        components, scene_key_index, scene_guid_index, summary = parse_scene(scene_path, cheat_idx)
        all_components.extend(components)
        scene_summaries.append(summary)
        for key, records in scene_key_index.items():
            key_index[key].extend(records)
        for guid, records in scene_guid_index.items():
            guid_index[guid].extend(records)

    out = {
        "format": "rogue-trader-scene-context.v1",
        "bundles_root": str(bundles_root),
        "glob": args.glob,
        "scene_count": len(scene_paths),
        "component_count": len(all_components),
        "string_key_count": len(key_index),
        "guid_count": len(guid_index),
        "scene_summaries": scene_summaries,
        "string_key_index": dict(sorted(key_index.items())),
        "guid_index": dict(sorted(guid_index.items())),
    }
    dump_json(args.out_json, out)
    print(json.dumps(
        {
            "scene_count": out["scene_count"],
            "component_count": out["component_count"],
            "string_key_count": out["string_key_count"],
            "guid_count": out["guid_count"],
        },
        ensure_ascii=False,
    ))


if __name__ == "__main__":
    main()
