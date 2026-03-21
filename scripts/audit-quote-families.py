import argparse
import json
import re
import subprocess
from collections import Counter
from pathlib import Path


ACTION_OPEN_QUOTE_RE = re.compile(r'^\([^)]*\)\s*"')
STAT_LIKE_OPEN_QUOTE_RE = re.compile(r'^(DC|ROLL|FC)\d+\s+[A-Za-z]+-".*')
ANY_OPEN_QUOTE_RE = re.compile(r'"')
CLOSING_QUOTE_RE = re.compile(r'".')


def fetch_rows(dsn, failed_only):
    where = "WHERE p.state = 'failed'" if failed_only else ""
    sql = f"""
SELECT i.id,
       p.state,
       p.last_error,
       COALESCE(i.pack_json->>'source_raw','') AS source_raw,
       COALESCE(i.pack_json->>'prev_en','') AS prev_en,
       COALESCE(i.pack_json->>'next_en','') AS next_en,
       COALESCE(i.pack_json->>'text_role','') AS text_role,
       COALESCE(i.pack_json->>'context_en','') AS context_en
FROM items i
LEFT JOIN pipeline_items p USING (id)
{where};
"""
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
        if len(parts) != 8:
            continue
        rows.append(
            {
                "id": parts[0],
                "state": parts[1],
                "last_error": parts[2],
                "source_raw": parts[3],
                "prev_en": parts[4],
                "next_en": parts[5],
                "text_role": parts[6],
                "context_en": parts[7],
            }
        )
    return rows


def classify(row):
    source = row["source_raw"].strip()
    next_en = row["next_en"].strip()
    if not source:
        return "other"
    if STAT_LIKE_OPEN_QUOTE_RE.match(source):
        return "stat_like_open_quote"
    if ACTION_OPEN_QUOTE_RE.match(source):
        return "action_open_quote"
    if ANY_OPEN_QUOTE_RE.search(source):
        if next_en and '"' in next_en:
            return "open_quote_other"
        if row["text_role"] in {"dialogue", "narration", "choice", "fragment"}:
            return "open_quote_other"
    return "other"


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--dsn", required=True)
    parser.add_argument("--out", required=True)
    parser.add_argument("--failed-only", action="store_true")
    args = parser.parse_args()

    rows = fetch_rows(args.dsn, args.failed_only)
    counts = Counter()
    families = []
    for row in rows:
        family = classify(row)
        counts[family] += 1
        if family != "other":
            families.append({**row, "family": family})

    report = {
        "failed_only": args.failed_only,
        "total_rows_scanned": len(rows),
        "family_counts": dict(counts),
        "examples": families[:200],
    }
    Path(args.out).write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps({"family_counts": report["family_counts"], "total_rows_scanned": report["total_rows_scanned"]}, ensure_ascii=False))


if __name__ == "__main__":
    main()
