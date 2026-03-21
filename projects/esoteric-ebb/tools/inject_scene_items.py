#!/usr/bin/env python3
"""
Inject scene overlay items into the live PostgreSQL pipeline.

Reads scene_overlay_package.json and inserts items into:
  - items table (pack_json + ko_json)
  - pipeline_items table (pending_translate state)

Then prints the command to route them to overlay-translate lane.

Usage:
    python inject_scene_items.py [--dry-run] [--dsn DSN]
"""

import argparse
import json
import os
import subprocess
import sys
from datetime import datetime, timezone

SOURCE_PATH = os.path.join(
    os.path.dirname(__file__), "..", "source", "scene_overlay_package.json"
)

PSQL = "C:/Program Files/PostgreSQL/17/bin/psql.exe"
DEFAULT_DSN = "postgres://postgres@127.0.0.1:5433/localize_agent?sslmode=disable"


def psql_exec(dsn: str, sql: str, dry_run: bool = False) -> str:
    if dry_run:
        print(f"  [DRY-RUN] {sql[:120]}...")
        return ""
    result = subprocess.run(
        [PSQL, dsn, "-t", "-A", "-c", sql],
        capture_output=True, timeout=30,
    )
    stdout = result.stdout.decode("utf-8", errors="replace").strip() if result.stdout else ""
    stderr = result.stderr.decode("utf-8", errors="replace").strip() if result.stderr else ""
    if result.returncode != 0:
        print(f"  ERROR: {stderr}", file=sys.stderr)
    return stdout


def psql_file(dsn: str, filepath: str) -> str:
    """Execute a SQL file via psql."""
    result = subprocess.run(
        [PSQL, dsn, "-f", filepath],
        capture_output=True, timeout=600,
    )
    stdout = result.stdout.decode("utf-8", errors="replace").strip() if result.stdout else ""
    stderr = result.stderr.decode("utf-8", errors="replace").strip() if result.stderr else ""
    if result.returncode != 0:
        print(f"  ERROR: {stderr}", file=sys.stderr)
    return stdout


def main():
    parser = argparse.ArgumentParser(description="Inject scene items into pipeline")
    parser.add_argument("--dry-run", action="store_true",
                        help="Print SQL without executing")
    parser.add_argument("--dsn", default=DEFAULT_DSN, help="PostgreSQL DSN")
    parser.add_argument("--input", default=None, help="Override input file path")
    args = parser.parse_args()

    input_path = args.input or os.path.normpath(SOURCE_PATH)
    if not os.path.isfile(input_path):
        print(f"ERROR: Input not found: {input_path}")
        sys.exit(1)

    with open(input_path, "r", encoding="utf-8") as f:
        package = json.load(f)

    items = package["items"]
    print(f"Loaded {len(items)} items from {input_path}")

    # Get current max ovl-mainmenu index
    max_id_str = psql_exec(args.dsn,
        "SELECT id FROM items WHERE id LIKE 'ovl-mainmenu-%' ORDER BY id DESC LIMIT 1;",
        dry_run=False)
    if max_id_str:
        current_max = int(max_id_str.split("-")[-1])
    else:
        current_max = -1
    print(f"Current max ovl-mainmenu index: {current_max}")

    # Get current max sort_index
    max_sort = psql_exec(args.dsn,
        "SELECT MAX(sort_index) FROM pipeline_items;",
        dry_run=False)
    sort_start = int(max_sort) + 1 if max_sort else 100000
    print(f"Sort index starting from: {sort_start}")

    now = datetime.now(timezone.utc).isoformat()
    new_index = current_max + 1
    inserted = 0
    skipped = 0

    # Check existing IDs to avoid duplicates
    existing_check = psql_exec(args.dsn,
        "SELECT id FROM items WHERE id LIKE 'ovl-mainmenu-%';",
        dry_run=False)
    existing_ids = set(existing_check.strip().split("\n")) if existing_check else set()

    # Build SQL file for batch execution
    sql_path = os.path.join(os.path.dirname(input_path), "inject_scene_items.sql")
    sql_lines = ["BEGIN;\n"]

    for item in items:
        ovl_id = f"ovl-mainmenu-{new_index:04d}"

        pack = {
            "id": ovl_id,
            "en": item["en"],
            "source_raw": item.get("source_raw", item["en"]),
            "current_ko": "",
            "fresh_ko": "",
            "context_en": item.get("context_en", ""),
            "text_role": item.get("text_role", "ui_label"),
            "speaker_hint": item.get("speaker_hint", ""),
            "scene_hint": item.get("scene_hint", ""),
            "source_file": item.get("source_file", ""),
            "source_type": item.get("source_type", "scene_m_text"),
            "translation_lane": item.get("translation_lane", "high"),
            "risk": item.get("risk", "low"),
            "meta_path_label": item.get("meta_path_label", ""),
            "prev_en": item.get("prev_en", ""),
            "next_en": item.get("next_en", ""),
            "prev_ko": "",
            "next_ko": "",
            "prev_line_id": "",
            "next_line_id": "",
            "segment_id": "",
            "segment_pos": "",
            "choice_prefix": "",
            "choice_mode": "",
            "choice_block_id": "",
            "is_stat_check": False,
            "stat_check": "",
            "resource_key": "",
            "notes": "",
            "retry_reason": "",
            "pipeline_version": "scene-extract-v1",
            "translation_policy": "",
            "proposed_ko_restored": "",
        }

        # Escape for SQL: double single quotes
        pack_json_str = json.dumps(pack, ensure_ascii=False).replace("'", "''")
        ko_json_str = json.dumps({"Text": ""}, ensure_ascii=False)

        sort_idx = sort_start + inserted

        sql_lines.append(
            f"INSERT INTO items(id, status, ko_json, pack_json, attempts, "
            f"last_error, updated_at, latency_ms, source_hash) "
            f"VALUES('{ovl_id}', 'new', '{ko_json_str}', "
            f"'{pack_json_str}', 0, '', '{now}', 0, '') "
            f"ON CONFLICT(id) DO NOTHING;\n"
        )
        sql_lines.append(
            f"INSERT INTO pipeline_items(id, sort_index, state, retry_count, "
            f"score_final, last_error, claimed_by, claimed_at, lease_until, updated_at) "
            f"VALUES('{ovl_id}', {sort_idx}, 'pending_translate', 0, "
            f"-1, '', '', NULL, NULL, '{now}') "
            f"ON CONFLICT(id) DO NOTHING;\n"
        )

        inserted += 1
        new_index += 1

    sql_lines.append("COMMIT;\n")

    print(f"Generated {inserted} items")
    print(f"ID range: ovl-mainmenu-{current_max + 1:04d} ~ ovl-mainmenu-{new_index - 1:04d}")

    if args.dry_run:
        print(f"\n[DRY-RUN] Would write SQL to: {sql_path}")
        print(f"First 3 INSERTs:")
        for line in sql_lines[1:7]:
            print(f"  {line[:120].strip()}")
        return

    with open(sql_path, "w", encoding="utf-8") as f:
        f.writelines(sql_lines)
    print(f"SQL file: {sql_path}")

    # Execute
    print(f"Executing against PostgreSQL...")
    result = psql_file(args.dsn, sql_path)
    if result:
        print(f"  psql output: {result[:200]}")

    # Verify
    count = psql_exec(args.dsn,
        "SELECT COUNT(*) FROM pipeline_items WHERE id LIKE 'ovl-mainmenu-%' AND state = 'pending_translate';")
    print(f"\nPending translate (ovl-mainmenu): {count}")

    total_ovl = psql_exec(args.dsn,
        "SELECT COUNT(*) FROM items WHERE id LIKE 'ovl-mainmenu-%';")
    print(f"Total ovl-mainmenu items: {total_ovl}")

    print(f"\nNext step - route to overlay lane:")
    print(f"  cd C:\\Users\\DELL\\Desktop\\localize-agent")
    print(f"  go run ./workflow/cmd/go-translation-pipeline --project-dir projects/esoteric-ebb --route-overlay-ui")


if __name__ == "__main__":
    main()
