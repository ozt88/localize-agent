import argparse
import json
import subprocess
from pathlib import Path


def fetch_rows(dsn):
    sql = """
SELECT i.id,
       COALESCE(i.pack_json->>'source_raw','') AS source_raw,
       COALESCE(i.pack_json->>'text_role','') AS text_role,
       COALESCE(i.pack_json->>'prev_en','') AS prev_en,
       COALESCE(i.pack_json->>'next_en','') AS next_en,
       COALESCE(i.pack_json->>'context_en','') AS context_en,
       COALESCE(i.pack_json->>'stat_check','') AS stat_check,
       COALESCE(i.pack_json->>'choice_mode','') AS choice_mode,
       COALESCE(i.pack_json->>'source_file','') AS source_file,
       COALESCE(i.pack_json->>'retry_reason','') AS retry_reason
FROM items i
JOIN pipeline_items p USING (id)
WHERE p.state = 'failed'
  AND p.last_error = 'translator produced no done row'
ORDER BY i.id;
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
        if len(parts) != 10:
            continue
        rows.append(
            {
                "id": parts[0],
                "source_raw": parts[1],
                "text_role": parts[2],
                "prev_en": parts[3],
                "next_en": parts[4],
                "context_en": parts[5],
                "stat_check": parts[6],
                "choice_mode": parts[7],
                "source_file": parts[8],
                "retry_reason": parts[9],
            }
        )
    return rows


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--dsn", required=True)
    parser.add_argument("--out", required=True)
    args = parser.parse_args()
    rows = fetch_rows(args.dsn)
    payload = {"count": len(rows), "rows": rows}
    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps({"count": len(rows), "out": str(out)}, ensure_ascii=False))


if __name__ == "__main__":
    main()
