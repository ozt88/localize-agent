import argparse
import json
import re
from collections import defaultdict
from pathlib import Path


TOKEN_RE = re.compile(r"([A-Za-z][A-Za-z0-9_\.]{2,}) ([0-9a-f]{32})")
CUE_NAME_RE = re.compile(r"^Cue_0*(\d+)$", re.IGNORECASE)
ANSWER_NAME_RE = re.compile(r"^Answer_0*(\d+)$", re.IGNORECASE)
ALT_ANSWER_RE = re.compile(r"^(.*)_answer_0*(\d+)$", re.IGNORECASE)
ALT_CUE_RE = re.compile(r"^(.*)_cue_0*(\d+)$", re.IGNORECASE)


def dump_json(path, obj):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(obj, ensure_ascii=False, indent=2), encoding="utf-8")


def normalize_name(name):
    base = name.strip()
    if base.endswith("_.Text"):
        base = base[:-6]
    elif base.endswith(".Text"):
        base = base[:-5]
    return base.rstrip("_.")


def is_dialog_anchor(name):
    lowered = name.lower()
    if lowered in {"blueprintdialog", "newblueprintdialog", "dialogseen", "startdialog"}:
        return False
    return lowered.endswith("_dialogue") or lowered.endswith("dialogue") or lowered.endswith("_dialog")


def is_linkable_node(name):
    lowered = name.lower()
    if CUE_NAME_RE.match(name) or ANSWER_NAME_RE.match(name):
        return True
    if ALT_ANSWER_RE.match(lowered) or ALT_CUE_RE.match(lowered):
        return True
    return False


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--bbp-file", required=True)
    ap.add_argument("--out-json", required=True)
    args = ap.parse_args()

    text = Path(args.bbp_file).read_bytes().decode("utf-8", errors="ignore")
    tokens = []
    for match in TOKEN_RE.finditer(text):
        tokens.append(
            {
                "pos": match.start(),
                "name": match.group(1),
                "guid32": match.group(2),
            }
        )

    anchors = [token for token in tokens if is_dialog_anchor(token["name"])]
    link_index = defaultdict(list)
    dialog_blocks = []

    for i, anchor in enumerate(anchors):
        start = anchor["pos"]
        end = anchors[i + 1]["pos"] if i + 1 < len(anchors) else len(text)
        block_tokens = [token for token in tokens if start <= token["pos"] < end]
        linked = []
        for token in block_tokens:
            if not is_linkable_node(token["name"]):
                continue
            linked.append(
                {
                    "name": token["name"],
                    "guid32": token["guid32"],
                }
            )
            link_index[normalize_name(token["name"])].append(
                {
                    "dialog_name": anchor["name"],
                    "dialog_guid32": anchor["guid32"],
                    "node_name": token["name"],
                    "node_guid32": token["guid32"],
                }
            )
        if linked:
            dialog_blocks.append(
                {
                    "dialog_name": anchor["name"],
                    "dialog_guid32": anchor["guid32"],
                    "linked_nodes": linked,
                }
            )

    out = {
        "format": "rogue-trader-bbp-dialogue-links.v1",
        "token_count": len(tokens),
        "dialog_anchor_count": len(anchors),
        "dialog_blocks": dialog_blocks,
        "node_link_index": dict(sorted(link_index.items())),
    }
    dump_json(args.out_json, out)
    print(
        json.dumps(
            {
                "token_count": len(tokens),
                "dialog_anchor_count": len(anchors),
                "linked_node_names": len(link_index),
                "dialog_blocks": len(dialog_blocks),
            },
            ensure_ascii=False,
        )
    )


if __name__ == "__main__":
    main()
