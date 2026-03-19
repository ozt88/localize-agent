"""
Context Cluster Builder
========================
translator_package_chunks.json에서 "여러 문장의 합으로 해석하면
번역 품질이 올라가는" 줄 그룹을 식별합니다.

5개 패턴:
  1. pronoun_ref     - 단문 대명사 참조 (≤6단어, 대명사 포함)
  2. speaker_change  - 화자 전환 단문 시퀀스
  3. incomplete      - 미완성/후행 대사 (..., -, unbalanced quote)
  4. action_reaction - 내레이션 후 단문 반응
  5. fragment_chain  - 기존 fragment/reaction 패턴

Usage:
    python build_context_clusters.py [--chunks PATH] [--output PATH]
"""

import argparse
import json
import re
import sys
from pathlib import Path

sys.stdout.reconfigure(encoding="utf-8", errors="replace")
sys.stderr.reconfigure(encoding="utf-8", errors="replace")

DEFAULT_CHUNKS = Path(__file__).parent.parent / "source" / "translator_package_chunks.json"
DEFAULT_OUTPUT = Path(__file__).parent.parent / "output" / "context_cluster_targets.json"

PRONOUNS = re.compile(
    r"\b(he|she|it|they|them|that|this|those|him|her|his|its|their|these)\b",
    re.IGNORECASE,
)

MAX_CLUSTER_LINES = 6


def word_count(text: str) -> int:
    return len(text.split())


def ends_incomplete(text: str) -> bool:
    text = text.rstrip()
    return (
        text.endswith("...")
        or text.endswith("-")
        or text.endswith("–")
        or text.endswith("—")
        or text.count('"') % 2 == 1
    )


def is_dialogue_like(role: str) -> bool:
    return role in ("dialogue", "reaction", "fragment", "choice")


def is_narration(role: str) -> bool:
    return role in ("narration",)


# ---------------------------------------------------------------------------
# Build a flat line index from chunks
# ---------------------------------------------------------------------------

def build_line_index(chunks_data: dict) -> tuple[list[dict], dict[str, int]]:
    """
    Returns:
      lines - flat list of line dicts (with parent chunk/segment info attached)
      idx   - line_id -> position in flat list
    """
    lines = []
    idx = {}
    for chunk in chunks_data.get("chunks", []):
        seg_id = chunk.get("parent_segment_id", "")
        source_file = chunk.get("source_file", "")
        scene_hint = chunk.get("scene_hint", "")
        for line in chunk.get("lines", []):
            entry = {
                "line_id": line["line_id"],
                "text": line.get("source_text", ""),
                "text_role": line.get("text_role", ""),
                "speaker_hint": line.get("speaker_hint") or "",
                "prev_line_id": line.get("prev_line_id"),
                "next_line_id": line.get("next_line_id"),
                "short_ctx": line.get("line_is_short_context_dependent", False),
                "segment_id": seg_id,
                "source_file": source_file,
                "scene_hint": scene_hint,
            }
            idx[entry["line_id"]] = len(lines)
            lines.append(entry)
    return lines, idx


def get_line(lines, idx, line_id):
    if line_id is None:
        return None
    pos = idx.get(line_id)
    if pos is None:
        return None
    return lines[pos]


# ---------------------------------------------------------------------------
# Pattern matchers - each returns a cluster dict or None
# ---------------------------------------------------------------------------

def match_pronoun_ref(line, lines, idx):
    """Pattern 1: 단문 대명사 참조"""
    if not is_dialogue_like(line["text_role"]):
        return None
    wc = word_count(line["text"])
    if wc > 6 or wc == 0:
        return None
    if not PRONOUNS.search(line["text"]):
        return None

    ids = []
    prev = get_line(lines, idx, line["prev_line_id"])
    if prev:
        ids.append(prev["line_id"])
    ids.append(line["line_id"])
    nxt = get_line(lines, idx, line["next_line_id"])
    if nxt:
        ids.append(nxt["line_id"])

    if len(ids) < 2:
        return None
    return {"pattern": "pronoun_ref", "ids": ids, "anchor": line["line_id"]}


def match_speaker_change(line, lines, idx):
    """Pattern 2: 화자 전환 단문 시퀀스"""
    if not is_dialogue_like(line["text_role"]):
        return None
    if not line["speaker_hint"]:
        return None

    # 앞뒤로 같은 세그먼트 내에서 화자가 바뀌는 연속 줄 탐색
    seq = [line]
    # backward
    cur = line
    while len(seq) < MAX_CLUSTER_LINES:
        prev = get_line(lines, idx, cur["prev_line_id"])
        if prev is None or prev["segment_id"] != line["segment_id"]:
            break
        if not is_dialogue_like(prev["text_role"]):
            break
        seq.insert(0, prev)
        cur = prev
    # forward
    cur = line
    while len(seq) < MAX_CLUSTER_LINES:
        nxt = get_line(lines, idx, cur["next_line_id"])
        if nxt is None or nxt["segment_id"] != line["segment_id"]:
            break
        if not is_dialogue_like(nxt["text_role"]):
            break
        seq.append(nxt)
        cur = nxt

    if len(seq) < 2:
        return None

    # 화자가 최소 2명 이상이어야
    speakers = {l["speaker_hint"] for l in seq if l["speaker_hint"]}
    if len(speakers) < 2:
        return None

    # 최소 1줄이 5단어 이하
    has_short = any(word_count(l["text"]) <= 5 for l in seq)
    if not has_short:
        return None

    ids = [l["line_id"] for l in seq]
    return {"pattern": "speaker_change", "ids": ids, "anchor": line["line_id"]}


def match_incomplete(line, lines, idx):
    """Pattern 3: 미완성/후행 대사"""
    role = line["text_role"]
    if role not in ("dialogue", "narration", "fragment"):
        return None
    wc = word_count(line["text"])
    if wc > 8 or wc == 0:
        return None
    if not ends_incomplete(line["text"]):
        return None

    ids = []
    prev = get_line(lines, idx, line["prev_line_id"])
    if prev:
        ids.append(prev["line_id"])
    ids.append(line["line_id"])
    nxt = get_line(lines, idx, line["next_line_id"])
    if nxt:
        ids.append(nxt["line_id"])

    if len(ids) < 2:
        return None
    return {"pattern": "incomplete", "ids": ids, "anchor": line["line_id"]}


def match_action_reaction(line, lines, idx):
    """Pattern 4: 내레이션 후 단문 반응"""
    if line["text_role"] not in ("dialogue", "reaction"):
        return None
    wc = word_count(line["text"])
    if wc > 4 or wc == 0:
        return None

    prev = get_line(lines, idx, line["prev_line_id"])
    if prev is None:
        return None
    if not is_narration(prev["text_role"]):
        return None

    ids = [prev["line_id"], line["line_id"]]
    return {"pattern": "action_reaction", "ids": ids, "anchor": line["line_id"]}


def match_fragment_chain(line, lines, idx):
    """Pattern 5: 기존 fragment/reaction 패턴"""
    if line["text_role"] not in ("fragment", "reaction"):
        return None
    if word_count(line["text"]) > 3 and line["text_role"] != "fragment":
        return None

    ids = []
    prev = get_line(lines, idx, line["prev_line_id"])
    if prev:
        ids.append(prev["line_id"])
    ids.append(line["line_id"])
    nxt = get_line(lines, idx, line["next_line_id"])
    if nxt:
        ids.append(nxt["line_id"])

    if len(ids) < 2:
        return None
    return {"pattern": "fragment_chain", "ids": ids, "anchor": line["line_id"]}


MATCHERS = [
    match_pronoun_ref,
    match_speaker_change,
    match_incomplete,
    match_action_reaction,
    match_fragment_chain,
]


# ---------------------------------------------------------------------------
# Merge overlapping clusters
# ---------------------------------------------------------------------------

def merge_clusters(clusters: list[dict]) -> list[dict]:
    """겹치는 ID를 가진 클러스터를 병합합니다."""
    # anchor 기준으로 deduplicate: 같은 anchor에서 여러 패턴이 매칭되면 첫 번째만 유지
    seen_anchors = {}
    deduped = []
    for c in clusters:
        anchor = c["anchor"]
        if anchor not in seen_anchors:
            seen_anchors[anchor] = c
            deduped.append(c)
        else:
            # 기존 클러스터에 패턴 추가
            existing = seen_anchors[anchor]
            if c["pattern"] not in existing.get("patterns", [existing["pattern"]]):
                if "patterns" not in existing:
                    existing["patterns"] = [existing["pattern"]]
                existing["patterns"].append(c["pattern"])
            # ID 확장
            for lid in c["ids"]:
                if lid not in existing["ids"]:
                    existing["ids"].append(lid)

    # 겹치는 ID 가진 클러스터 병합
    merged = []
    used = set()
    for i, c in enumerate(deduped):
        if i in used:
            continue
        group_ids = list(c["ids"])
        group_patterns = c.get("patterns", [c["pattern"]])
        group_anchor = c["anchor"]
        # 다른 클러스터와 겹침 확인
        changed = True
        while changed:
            changed = False
            for j, other in enumerate(deduped):
                if j in used or j == i:
                    continue
                if set(group_ids) & set(other["ids"]):
                    for lid in other["ids"]:
                        if lid not in group_ids:
                            group_ids.append(lid)
                    for p in other.get("patterns", [other["pattern"]]):
                        if p not in group_patterns:
                            group_patterns.append(p)
                    used.add(j)
                    changed = True

        used.add(i)
        # 최대 크기 제한
        if len(group_ids) > MAX_CLUSTER_LINES:
            group_ids = group_ids[:MAX_CLUSTER_LINES]

        merged.append({
            "cluster_id": f"ctx-{len(merged):05d}",
            "patterns": group_patterns,
            "anchor": group_anchor,
            "ids": group_ids,
            "line_count": len(group_ids),
        })

    return merged


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(description="Build context clusters")
    parser.add_argument("--chunks", default=str(DEFAULT_CHUNKS), help="translator_package_chunks.json path")
    parser.add_argument("--output", default=str(DEFAULT_OUTPUT), help="output path")
    args = parser.parse_args()

    print("=== Context Cluster Builder ===\n")

    # Load chunks
    print(f"Loading chunks from {args.chunks}...")
    with open(args.chunks, "r", encoding="utf-8") as f:
        chunks_data = json.load(f)

    lines, idx = build_line_index(chunks_data)
    print(f"  {len(lines)} lines indexed from {len(chunks_data.get('chunks', []))} chunks.\n")

    # Match patterns
    all_clusters = []
    pattern_counts = {}
    for line in lines:
        for matcher in MATCHERS:
            result = matcher(line, lines, idx)
            if result:
                all_clusters.append(result)
                p = result["pattern"]
                pattern_counts[p] = pattern_counts.get(p, 0) + 1

    print(f"Raw matches: {len(all_clusters)}")
    for p, count in sorted(pattern_counts.items(), key=lambda x: -x[1]):
        print(f"  {p:20s} {count:6d}")

    # Merge
    print(f"\nMerging overlapping clusters...")
    merged = merge_clusters(all_clusters)
    print(f"  {len(merged)} clusters after merge.\n")

    # Stats
    size_dist = {}
    pattern_dist = {}
    for c in merged:
        sz = c["line_count"]
        size_dist[sz] = size_dist.get(sz, 0) + 1
        for p in c["patterns"]:
            pattern_dist[p] = pattern_dist.get(p, 0) + 1

    print("Size distribution:")
    for sz in sorted(size_dist):
        print(f"  {sz} lines: {size_dist[sz]:6d} clusters")

    print("\nPattern distribution (after merge):")
    for p, count in sorted(pattern_dist.items(), key=lambda x: -x[1]):
        print(f"  {p:20s} {count:6d}")

    # Enrich with text
    for c in merged:
        c["lines"] = []
        for lid in c["ids"]:
            pos = idx.get(lid)
            if pos is not None:
                l = lines[pos]
                c["lines"].append({
                    "line_id": lid,
                    "text": l["text"],
                    "text_role": l["text_role"],
                    "speaker_hint": l["speaker_hint"],
                })
                if "segment_id" not in c:
                    c["segment_id"] = l["segment_id"]
                    c["source_file"] = l["source_file"]
                    c["scene_hint"] = l["scene_hint"]
        c["joined_en"] = " / ".join(ln["text"] for ln in c["lines"])

    # Save
    output_path = Path(args.output)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    with open(output_path, "w", encoding="utf-8") as f:
        json.dump(merged, f, ensure_ascii=False, indent=2)

    print(f"\nSaved {len(merged)} clusters to {output_path}")


if __name__ == "__main__":
    main()
