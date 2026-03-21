#!/usr/bin/env python3
"""
Ingest untranslated_capture.json into the pipeline DB.

Deduplicates against existing items by source_raw text.
If an existing item already has a translation, reuses it.
Only creates new pipeline items for truly new source texts.

Usage:
    python ingest_untranslated.py --input <capture.json> [--dry-run]
"""
import argparse
import json
import hashlib
import os
import re
import subprocess
import sys

PSQL = "C:/Program Files/PostgreSQL/17/bin/psql.exe"
DSN_ARGS = ["-h", "127.0.0.1", "-p", "5433", "-U", "postgres", "-d", "localize_agent"]

SKIP_PATTERNS = [
    r"^\d{4}-\d{2}-\d{2}",       # timestamps
    r"^Day \d+",                  # day markers
    r"^VSYNC",                    # settings
    r"^(Chronus|Eltre|Felteni|Ingarmi|March|PW,) \d+",  # game dates
]

SKIP_EXACT = {
    "Averia Serif", "EB Garamond", "Josefin Sans", "Open Dyslexic", "PT Serif",
}


def psql_query(sql):
    env = {**os.environ, "PGCLIENTENCODING": "UTF8"}
    r = subprocess.run(
        [PSQL] + DSN_ARGS + ["-t", "-A", "-c", sql],
        capture_output=True, text=True, encoding="utf-8", env=env, timeout=30,
    )
    return r.stdout.strip()


def psql_file(path):
    env = {**os.environ, "PGCLIENTENCODING": "UTF8"}
    r = subprocess.run(
        [PSQL] + DSN_ARGS + ["-f", path],
        capture_output=True, text=True, encoding="utf-8", env=env, timeout=60,
    )
    return r.stdout.count("INSERT"), r.stdout.count("UPDATE")


def should_skip(src):
    if not src or len(src) <= 2:
        return True
    if src in SKIP_EXACT:
        return True
    for pat in SKIP_PATTERNS:
        if re.match(pat, src):
            return True
    return False


def main():
    parser = argparse.ArgumentParser(description="Ingest untranslated capture")
    parser.add_argument("--input", required=True, help="Path to untranslated_capture.json")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()

    with open(args.input, "r", encoding="utf-8") as f:
        data = json.load(f)

    entries = data.get("entries", [])
    print(f"Capture entries: {len(entries)}")

    # Filter
    candidates = []
    for e in entries:
        src = e["source"].strip()
        if not should_skip(src):
            candidates.append(src)

    print(f"After filtering: {len(candidates)}")

    # Load ALL existing source_raw -> (id, ko) from DB
    rows = psql_query(
        "SELECT i.id, i.pack_json->>'source_raw', COALESCE(i.ko_json->>'Text', '') "
        "FROM items i"
    )
    existing_by_source = {}
    for line in rows.split("\n"):
        if not line:
            continue
        parts = line.split("|", 2)
        if len(parts) == 3:
            src = parts[1]
            if src not in existing_by_source:
                existing_by_source[src] = {"id": parts[0], "ko": parts[2]}

    # Classify
    already_exists = 0
    truly_new = []
    for src in candidates:
        if src in existing_by_source:
            already_exists += 1
        else:
            truly_new.append(src)

    print(f"Already in DB (skipped): {already_exists}")
    print(f"Truly new items: {len(truly_new)}")

    if not truly_new:
        print("Nothing to add.")
        return

    if args.dry_run:
        print("\n[DRY-RUN] Would add:")
        for src in truly_new[:10]:
            print(f"  {src[:80]}")
        if len(truly_new) > 10:
            print(f"  ... +{len(truly_new) - 10} more")
        return

    # Get max sort_index
    max_sort = int(psql_query("SELECT COALESCE(MAX(sort_index),0) FROM pipeline_items") or "0")
    now = "2026-03-22T06:00:00+09:00"

    sql_lines = ["SET client_encoding = 'UTF8';", "BEGIN;"]
    for i, src in enumerate(truly_new):
        h = hashlib.md5(src.encode("utf-8")).hexdigest()[:12]
        item_id = f"rt-{h}"
        safe_src = src.replace("'", "''")
        pack = json.dumps({
            "source_raw": src,
            "current_ko": "",
            "fresh_ko": "",
            "text_role": "runtime_capture",
            "source_type": "runtime_capture",
        }, ensure_ascii=False).replace("'", "''")
        ko_json = json.dumps({"Text": ""}, ensure_ascii=False)

        sql_lines.append(
            f"INSERT INTO items(id, status, ko_json, pack_json, attempts, last_error, "
            f"updated_at, latency_ms, source_hash) "
            f"VALUES('{item_id}', 'new', '{ko_json}', '{pack}', 0, '', '{now}', 0, '') "
            f"ON CONFLICT(id) DO NOTHING;"
        )
        sql_lines.append(
            f"INSERT INTO pipeline_items(id, sort_index, state, retry_count, score_final, "
            f"last_error, claimed_by, claimed_at, lease_until, updated_at) "
            f"VALUES('{item_id}', {max_sort + i + 1}, 'pending_translate', 0, -1, '', '', "
            f"NULL, NULL, '{now}') ON CONFLICT(id) DO NOTHING;"
        )

    sql_lines.append("COMMIT;")

    sql_path = os.path.join(os.path.dirname(args.input), "ingest_untranslated.sql")
    with open(sql_path, "w", encoding="utf-8") as f:
        f.write("\n".join(sql_lines))

    inserts, updates = psql_file(sql_path)
    print(f"DB inserts: {inserts}")


if __name__ == "__main__":
    main()
