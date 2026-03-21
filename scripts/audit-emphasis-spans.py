import argparse
import json
import re
import subprocess
from collections import Counter
from pathlib import Path


EMPH_RE = re.compile(r"<(i|b)>(.*?)</\1>", re.IGNORECASE | re.DOTALL)
PUNCT_STRIP_RE = re.compile(r"[\s\.,!?;:'\"“”‘’()\[\]{}<>…\-]+", re.UNICODE)


def extract_spans(text):
    if not text:
        return []
    spans = []
    for match in EMPH_RE.finditer(text):
        tag = match.group(1).lower()
        inner = match.group(2)
        spans.append(
            {
                "tag": tag,
                "text": inner,
                "norm": normalize(inner),
                "start": match.start(),
                "end": match.end(),
            }
        )
    return spans


def normalize(text):
    return PUNCT_STRIP_RE.sub("", text or "").lower()


def classify(source_spans, output_spans):
    if not source_spans and not output_spans:
        return None
    if source_spans and not output_spans:
        return "missing_emphasis"
    if output_spans and not source_spans:
        return "extra_emphasis"
    if len(source_spans) != len(output_spans):
        return "span_count_mismatch"
    for src, out in zip(source_spans, output_spans):
        if src["tag"] != out["tag"]:
            return "tag_type_drift"
        if src["norm"] != out["norm"]:
            return "span_drift"
    return None


def load_source_map(path):
    data = json.loads(Path(path).read_text(encoding="utf-8"))
    if "strings" not in data:
        raise ValueError(f"unsupported source format: {path}")
    out = {}
    for line_id, obj in data["strings"].items():
        if isinstance(obj, dict):
            out[line_id] = obj.get("Text", "")
    return out


def fetch_outputs(dsn):
    sql = (
        "SELECT id, COALESCE(pack_json->>'source_raw',''), "
        "COALESCE(ko_json->>'Text',''), COALESCE(pack_json->>'fresh_ko','') "
        "FROM items"
    )
    cmd = [
        r"C:\Program Files\PostgreSQL\17\bin\psql.exe",
        dsn,
        "-t",
        "-A",
        "-F",
        "\t",
        "-P",
        "pager=off",
        "-c",
        sql,
    ]
    proc = subprocess.run(cmd, capture_output=True, text=True, encoding="utf-8", check=True)
    rows = []
    for line in proc.stdout.splitlines():
        if not line.strip():
            continue
        parts = line.split("\t")
        if len(parts) != 4:
            continue
        rows.append(
            {
                "id": parts[0],
                "source_raw": parts[1],
                "ko_text": parts[2],
                "fresh_ko": parts[3],
            }
        )
    return rows


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--source-json", required=True)
    parser.add_argument("--dsn", required=True)
    parser.add_argument("--out", required=True)
    args = parser.parse_args()

    source_map = load_source_map(args.source_json)
    rows = fetch_outputs(args.dsn)

    mismatches = []
    counts = Counter()
    for row in rows:
        source_raw = row["source_raw"] or source_map.get(row["id"], "")
        source_spans = extract_spans(source_raw)
        if not source_spans and "<i>" not in row["ko_text"] and "<b>" not in row["ko_text"] and "<i>" not in row["fresh_ko"] and "<b>" not in row["fresh_ko"]:
            continue
        output_text = row["ko_text"] or row["fresh_ko"]
        output_spans = extract_spans(output_text)
        mismatch = classify(source_spans, output_spans)
        if mismatch is None:
            continue
        counts[mismatch] += 1
        mismatches.append(
            {
                "id": row["id"],
                "mismatch_type": mismatch,
                "source_raw": source_raw,
                "source_spans": [{"tag": s["tag"], "text": s["text"]} for s in source_spans],
                "output_text": output_text,
                "output_spans": [{"tag": s["tag"], "text": s["text"]} for s in output_spans],
                "fresh_ko": row["fresh_ko"],
                "ko_text": row["ko_text"],
            }
        )

    report = {
        "source_json": args.source_json,
        "total_rows_scanned": len(rows),
        "mismatch_counts": dict(counts),
        "mismatch_total": sum(counts.values()),
        "examples": mismatches[:100],
    }
    Path(args.out).write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps({"mismatch_total": report["mismatch_total"], "mismatch_counts": report["mismatch_counts"]}, ensure_ascii=False))


if __name__ == "__main__":
    main()
