import argparse
import json
from collections import Counter, defaultdict
from pathlib import Path


def bucket(row):
    src = row.get("source_raw", "")
    role = row.get("text_role", "") or "no_role"
    score = row.get("score_final", "")
    try:
        score_val = float(score)
    except Exception:
        score_val = -1
    bits = [role]
    if len(src) > 140:
        bits.append("very_long")
    elif len(src) > 80:
        bits.append("long")
    if "<i>" in src or "<b>" in src:
        bits.append("rich")
    if ' - ' in src:
        bits.append("dash")
    if '"' in src:
        bits.append("quote")
    if row.get("stat_check"):
        bits.append("stat")
    if score_val >= 0:
        if score_val < 20:
            bits.append("score_lt20")
        elif score_val < 40:
            bits.append("score_lt40")
        elif score_val < 60:
            bits.append("score_lt60")
        else:
            bits.append("score_lt70")
    return "|".join(bits)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--fixture", required=True)
    parser.add_argument("--out")
    args = parser.parse_args()

    payload = json.loads(Path(args.fixture).read_text(encoding="utf-8"))
    rows = payload["rows"]
    counts = Counter()
    examples = defaultdict(list)
    for row in rows:
        b = bucket(row)
        counts[b] += 1
        if len(examples[b]) < 4:
            examples[b].append(row)

    report = {
        "count": len(rows),
        "top_buckets": counts.most_common(20),
        "examples": {k: examples[k] for k, _ in counts.most_common(10)},
    }
    rendered = json.dumps(report, ensure_ascii=False, indent=2)
    if args.out:
        Path(args.out).write_text(rendered, encoding="utf-8")
        print(json.dumps({"count": len(rows), "out": str(Path(args.out))}, ensure_ascii=False))
        return
    print(rendered)


if __name__ == "__main__":
    main()
