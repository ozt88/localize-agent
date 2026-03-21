#!/usr/bin/env python3
"""
Core module for ingesting new translation items into the pipeline.

All tools that add items to the pipeline should use this module instead of
writing their own DB insertion and source file update logic.

Responsibilities:
  - Validate and filter items (skip non-translatable content)
  - Generate sequential pipeline IDs
  - Insert into PostgreSQL items + pipeline_items tables
  - Update source_esoteric.json, ids_esoteric.txt, current_esoteric.json
  - Report results

Usage as library:
    from pipeline_ingest import PipelineIngest

    ingest = PipelineIngest(batch_dir="projects/esoteric-ebb/output/batches/canonical_full_retranslate_live")
    ingest.add(items)           # list of dicts with at minimum {id, source}
    result = ingest.apply()     # inserts to DB + updates source files
    result = ingest.dry_run()   # preview without changes

Usage as CLI:
    python pipeline_ingest.py --input items.json [--apply] [--dry-run]
"""

import argparse
import json
import os
import re
import subprocess
import sys
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path

PSQL = "C:/Program Files/PostgreSQL/17/bin/psql.exe"
DEFAULT_DSN_ARGS = ["-h", "127.0.0.1", "-p", "5433", "-U", "postgres", "-d", "localize_agent"]

PROJECT_DIR = Path(__file__).parent.parent
DEFAULT_BATCH_DIR = PROJECT_DIR / "output" / "batches" / "canonical_full_retranslate_live"

# ─── Pack JSON schema ───
# Every item inserted into the `items` table must have these fields in pack_json.
# Missing fields default to empty string / False.
PACK_SCHEMA = [
    "source_raw",           # original English text
    "current_ko",           # existing Korean translation (empty for new)
    "fresh_ko",             # newly translated Korean (empty initially)
    "text_role",            # dialogue, narration, choice, ui_label, tooltip, etc.
    "source_type",          # origin: textasset_update, overlay_update, scene_m_text, etc.
    "context_en",           # surrounding English text for translation context
    "speaker_hint",         # character name if dialogue
    "prev_line_id",         # ID of previous line for ordering
    "next_line_id",         # ID of next line for ordering
    "prev_en",              # previous line English text
    "next_en",              # next line English text
    "source_file",          # origin file name
    "meta_path_label",      # UI path or scene path
    "scene_hint",           # scene name
    "translation_lane",     # high / low
    "risk",                 # low / medium / high
    "resource_key",         # game resource key
    "retry_reason",         # reason for re-translation
]

# ─── Filtering ───

SKIP_PATTERNS = [
    re.compile(r"^[A-Za-z0-9_.\-]+(\s*\(\d+\))?$"),   # 3D model/bone names
    re.compile(r"^[\d.\-\s,]+$"),                        # pure numbers
    re.compile(r"^(PlaySFX|StopMusic|StopAmbiance|FadeTo|UpdateEntities|RollLock)"),
    re.compile(r"^DC\d+$"),
    re.compile(r"^[A-Z]{2,5}\d*$"),                      # short codes
]

SKIP_EXACT = frozenset({
    "...", "SPELLNAME", "SPELL DESC", "ItemName",
    "Item descriptions here.", "No.", "Yes.",
})


def is_translatable(text: str) -> bool:
    """Check if a text string should be translated."""
    text = text.strip()
    if len(text) <= 2:
        return False
    if text in SKIP_EXACT:
        return False
    for pat in SKIP_PATTERNS:
        if pat.match(text):
            return False
    return True


# ─── Data types ───

@dataclass
class IngestItem:
    """A single item to ingest into the pipeline."""
    id: str
    source: str
    text_role: str = "dialogue"
    source_type: str = ""
    context_en: str = ""
    speaker_hint: str = ""
    prev_line_id: str = ""
    next_line_id: str = ""
    prev_en: str = ""
    next_en: str = ""
    source_file: str = ""
    meta_path_label: str = ""
    scene_hint: str = ""
    translation_lane: str = "high"
    risk: str = "low"
    resource_key: str = ""
    current_ko: str = ""

    def to_pack_json(self) -> dict:
        """Build pack_json dict for DB insertion."""
        pack = {"source_raw": self.source, "fresh_ko": ""}
        for key in PACK_SCHEMA:
            if key in ("source_raw", "fresh_ko"):
                continue
            pack[key] = getattr(self, key, "") or ""
        return pack

    @classmethod
    def from_dict(cls, d: dict) -> "IngestItem":
        """Create from a plain dict (JSON input)."""
        known = {f.name for f in cls.__dataclass_fields__.values()}
        kwargs = {k: v for k, v in d.items() if k in known}
        return cls(**kwargs)


@dataclass
class IngestResult:
    """Result of an ingest operation."""
    total_input: int = 0
    filtered_out: int = 0
    already_exists: int = 0
    inserted_items: int = 0
    inserted_pipeline: int = 0
    source_files_updated: int = 0
    errors: list = field(default_factory=list)

    def summary(self) -> str:
        lines = [
            f"Input: {self.total_input}",
            f"Filtered (non-translatable): {self.filtered_out}",
            f"Already in DB: {self.already_exists}",
            f"Inserted to items table: {self.inserted_items}",
            f"Inserted to pipeline_items: {self.inserted_pipeline}",
            f"Source files updated: {self.source_files_updated} new IDs",
        ]
        if self.errors:
            lines.append(f"Errors: {len(self.errors)}")
            for e in self.errors[:5]:
                lines.append(f"  {e}")
        return "\n".join(lines)


# ─── Core ingest class ───

class PipelineIngest:
    """Manages ingestion of new items into the translation pipeline."""

    def __init__(
        self,
        batch_dir: str | Path = DEFAULT_BATCH_DIR,
        dsn_args: list[str] | None = None,
        psql_path: str = PSQL,
    ):
        self.batch_dir = Path(batch_dir)
        self.dsn_args = dsn_args or DEFAULT_DSN_ARGS
        self.psql = psql_path
        self._items: list[IngestItem] = []

    def add(self, items: list[dict | IngestItem], filter_translatable: bool = True):
        """Add items to the ingest queue.

        Args:
            items: list of dicts or IngestItem objects.
                   Dicts must have at minimum 'id' and 'source' keys.
            filter_translatable: if True, skip non-translatable items.
        """
        for item in items:
            if isinstance(item, dict):
                item = IngestItem.from_dict(item)
            if filter_translatable and not is_translatable(item.source):
                continue
            self._items.append(item)

    def add_raw(self, items: list[IngestItem]):
        """Add items without filtering."""
        self._items.extend(items)

    @property
    def pending_count(self) -> int:
        return len(self._items)

    def dry_run(self) -> IngestResult:
        """Preview what would happen without making changes."""
        result = IngestResult(total_input=len(self._items))
        existing = self._get_existing_ids()
        for item in self._items:
            if item.id in existing:
                result.already_exists += 1
            else:
                result.inserted_items += 1
                result.inserted_pipeline += 1
                result.source_files_updated += 1
        return result

    def apply(self, initial_state: str = "pending_translate") -> IngestResult:
        """Insert items into DB and update source files.

        Args:
            initial_state: pipeline state for new items.
                           Use 'pending_translate' for fresh items,
                           'pending_overlay_translate' for overlay items.
        """
        result = IngestResult(total_input=len(self._items))
        if not self._items:
            return result

        # 1. Check existing IDs
        existing = self._get_existing_ids()
        new_items = []
        for item in self._items:
            if item.id in existing:
                result.already_exists += 1
            else:
                new_items.append(item)

        if not new_items:
            return result

        # 2. Generate SQL and insert into items table
        sql_path = self.batch_dir / "_ingest_items.sql"
        self._generate_items_sql(new_items, sql_path)
        if not self._exec_sql_file(sql_path):
            result.errors.append("Failed to insert into items table")
            return result
        result.inserted_items = len(new_items)

        # 3. Insert into pipeline_items table
        pipeline_sql_path = self.batch_dir / "_ingest_pipeline.sql"
        self._generate_pipeline_sql(new_items, initial_state, pipeline_sql_path)
        if not self._exec_sql_file(pipeline_sql_path):
            result.errors.append("Failed to insert into pipeline_items table")
            return result
        result.inserted_pipeline = len(new_items)

        # 4. Update source files
        result.source_files_updated = self._update_source_files(new_items)

        # 5. Cleanup temp SQL
        sql_path.unlink(missing_ok=True)
        pipeline_sql_path.unlink(missing_ok=True)

        return result

    # ─── Private helpers ───

    def _get_existing_ids(self) -> set[str]:
        """Query DB for existing item IDs."""
        id_list = [item.id for item in self._items]
        if not id_list:
            return set()
        # Batch query in chunks
        existing = set()
        for i in range(0, len(id_list), 500):
            chunk = id_list[i : i + 500]
            in_clause = ",".join(f"'{x.replace(chr(39), chr(39)+chr(39))}'" for x in chunk)
            result = self._exec_sql(f"SELECT id FROM items WHERE id IN ({in_clause});")
            if result:
                existing.update(line.strip() for line in result.splitlines() if line.strip())
        return existing

    def _generate_items_sql(self, items: list[IngestItem], sql_path: Path):
        """Generate SQL to insert into items table."""
        lines = ["SET client_encoding = 'UTF8';"]
        now = datetime.now(timezone.utc).isoformat()
        for item in items:
            safe_id = item.id.replace("'", "''")
            pack = json.dumps(item.to_pack_json(), ensure_ascii=False).replace("'", "''")
            ko = json.dumps({"Text": ""}, ensure_ascii=False).replace("'", "''")
            lines.append(
                f"INSERT INTO items (id, status, ko_json, pack_json, attempts, "
                f"last_error, updated_at, latency_ms, source_hash) "
                f"VALUES ('{safe_id}', 'pending', '{ko}'::jsonb, '{pack}'::jsonb, "
                f"0, '', '{now}', 0, '') ON CONFLICT (id) DO NOTHING;"
            )
        sql_path.write_text("\n".join(lines), encoding="utf-8")

    def _generate_pipeline_sql(
        self, items: list[IngestItem], state: str, sql_path: Path
    ):
        """Generate SQL to insert into pipeline_items table."""
        # Get max sort_index
        max_sort_str = self._exec_sql("SELECT COALESCE(MAX(sort_index), 0) FROM pipeline_items;")
        sort_start = int(max_sort_str.strip()) + 1 if max_sort_str and max_sort_str.strip().isdigit() else 100000
        now = datetime.now(timezone.utc).isoformat()

        lines = ["SET client_encoding = 'UTF8';"]
        for idx, item in enumerate(items):
            safe_id = item.id.replace("'", "''")
            sort_idx = sort_start + idx
            lines.append(
                f"INSERT INTO pipeline_items (id, sort_index, state, retry_count, "
                f"score_final, last_error, claimed_by, claimed_at, lease_until, updated_at) "
                f"VALUES ('{safe_id}', {sort_idx}, '{state}', 0, "
                f"-1, '', '', NULL, NULL, '{now}') ON CONFLICT (id) DO NOTHING;"
            )
        sql_path.write_text("\n".join(lines), encoding="utf-8")

    def _update_source_files(self, items: list[IngestItem]) -> int:
        """Append new items to source_esoteric.json, ids_esoteric.txt, current_esoteric.json."""
        source_path = self.batch_dir / "source_esoteric.json"
        ids_path = self.batch_dir / "ids_esoteric.txt"
        current_path = self.batch_dir / "current_esoteric.json"

        if not source_path.exists():
            return 0

        source = json.loads(source_path.read_text(encoding="utf-8"))
        existing_ids = [
            l.strip()
            for l in ids_path.read_text(encoding="utf-8").splitlines()
            if l.strip()
        ]
        current = json.loads(current_path.read_text(encoding="utf-8"))

        strings = source.get("strings", source)
        cur_strings = current.get("strings", current)
        existing_set = set(existing_ids)
        added = 0

        for item in items:
            if item.id not in existing_set:
                strings[item.id] = {"Text": item.source}
                cur_strings[item.id] = {"Text": item.current_ko or ""}
                existing_ids.append(item.id)
                existing_set.add(item.id)
                added += 1

        source_path.write_text(json.dumps(source, ensure_ascii=False), encoding="utf-8")
        ids_path.write_text("\n".join(existing_ids) + "\n", encoding="utf-8")
        current_path.write_text(
            json.dumps(current, ensure_ascii=False), encoding="utf-8"
        )
        return added

    def _exec_sql(self, sql: str) -> str:
        """Execute a SQL query and return stdout."""
        env = {**os.environ, "PGCLIENTENCODING": "UTF8"}
        result = subprocess.run(
            [self.psql, *self.dsn_args, "-t", "-A", "-c", sql],
            capture_output=True,
            text=True,
            encoding="utf-8",
            env=env,
            timeout=30,
        )
        return result.stdout.strip() if result.stdout else ""

    def _exec_sql_file(self, path: Path) -> bool:
        """Execute a SQL file and return success."""
        env = {**os.environ, "PGCLIENTENCODING": "UTF8"}
        result = subprocess.run(
            [self.psql, *self.dsn_args, "-f", str(path)],
            capture_output=True,
            text=True,
            encoding="utf-8",
            env=env,
            timeout=600,
        )
        if result.returncode != 0:
            print(f"  SQL error: {result.stderr[:300]}", file=sys.stderr)
            return False
        return True


# ─── CLI ───

def main():
    parser = argparse.ArgumentParser(
        description="Ingest translation items into the pipeline"
    )
    parser.add_argument(
        "--input", required=True, help="JSON file with items (list of {id, source, ...})"
    )
    parser.add_argument(
        "--batch-dir",
        default=str(DEFAULT_BATCH_DIR),
        help="Pipeline batch directory",
    )
    parser.add_argument(
        "--state",
        default="pending_translate",
        choices=[
            "pending_translate",
            "pending_overlay_translate",
            "pending_score",
        ],
        help="Initial pipeline state for new items",
    )
    parser.add_argument("--apply", action="store_true", help="Apply changes")
    parser.add_argument(
        "--dry-run", action="store_true", help="Preview without changes"
    )
    parser.add_argument(
        "--no-filter",
        action="store_true",
        help="Skip translatable filtering (insert all items)",
    )
    args = parser.parse_args()

    input_path = Path(args.input)
    if not input_path.exists():
        print(f"Error: input not found: {input_path}", file=sys.stderr)
        sys.exit(1)

    items = json.loads(input_path.read_text(encoding="utf-8"))
    print(f"Loaded {len(items)} items from {input_path}")

    ingest = PipelineIngest(batch_dir=args.batch_dir)
    ingest.add(items, filter_translatable=not args.no_filter)
    print(f"After filtering: {ingest.pending_count} items")

    if args.dry_run:
        result = ingest.dry_run()
        print(f"\n[dry-run]\n{result.summary()}")
        return

    if args.apply:
        result = ingest.apply(initial_state=args.state)
        print(f"\n{result.summary()}")
        if not result.errors:
            print("\nDone. Pipeline will pick up new items automatically.")
    else:
        print(f"\nTo apply: python {__file__} --input {args.input} --apply")


if __name__ == "__main__":
    main()
