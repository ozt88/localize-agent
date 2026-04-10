#!/usr/bin/env python3
"""
Prebuild RAG context for batch-level translation.

Reads all pipeline_items from PostgreSQL, groups by batch_id, and matches
enriched termbank entries using word-boundary regex (mirroring lore.go
containsLoreTerm). Outputs rag_batch_context.json mapping batch_id to
top-3 world-building hints.

Usage:
    python projects/esoteric-ebb/rag/prebuild_rag_context.py
"""

import json
import os
import re
import subprocess
import sys
from dataclasses import dataclass, field
from pathlib import Path

# --- Constants ---

PSQL = "C:/Program Files/PostgreSQL/17/bin/psql.exe"
DEFAULT_DSN_ARGS = ["-h", "127.0.0.1", "-p", "5433", "-U", "postgres", "-d", "localize_agent"]

SCRIPT_DIR = Path(__file__).resolve().parent
ENRICHED_TERMBANK_PATH = SCRIPT_DIR / "enriched_termbank.json"
OUTPUT_PATH = SCRIPT_DIR / "rag_batch_context.json"

MAX_HINTS = 3

# Category priority for tie-breaking when >3 hits (lower = higher priority)
CATEGORY_PRIORITY = {
    "character": 0,
    "location": 1,
    "lore": 2,
    "item": 3,
    "glossary": 4,
}
DEFAULT_PRIORITY = 5


# --- Data classes ---

@dataclass
class TermEntry:
    term: str
    description: str
    category: str
    aliases: list = field(default_factory=list)


# --- Word-boundary matching (mirrors lore.go containsLoreTerm) ---

def contains_term(text: str, term: str) -> bool:
    """Check if text contains term with word boundaries and English suffixes."""
    if len(term) < 3:
        return False
    suffix = r"(?:'s|s|ed|ing)?"
    pattern = rf"(?i)(?:^|[^A-Za-z0-9]){re.escape(term)}{suffix}(?:[^A-Za-z0-9]|$)"
    return bool(re.search(pattern, text))


def category_priority(cat: str) -> int:
    """Return sort priority for a category string. Handles multi-category like 'language, politics'."""
    cats = [c.strip().lower() for c in cat.split(",")]
    return min(CATEGORY_PRIORITY.get(c, DEFAULT_PRIORITY) for c in cats)


def match_rag_hints(entries: list[TermEntry], batch_text: str, max_hints: int = MAX_HINTS) -> list[dict]:
    """Match termbank entries against batch text, return top-N hints."""
    hits = []
    seen: set[str] = set()

    for entry in entries:
        terms = [entry.term] + entry.aliases
        for term in terms:
            term = term.strip()
            if not term or len(term) < 3:
                continue
            if contains_term(batch_text, term):
                if entry.term not in seen:
                    seen.add(entry.term)
                    hits.append({
                        "term": entry.term,
                        "description": entry.description,
                        "category": entry.category,
                    })
                break

    # Sort: category priority first, then by term length descending (more specific first)
    hits.sort(key=lambda h: (category_priority(h["category"]), -len(h["term"])))
    return hits[:max_hints]


# --- DB access ---

def fetch_batch_texts(psql: str = PSQL, dsn_args: list[str] | None = None) -> dict[str, str]:
    """Query PostgreSQL for batch_id -> concatenated source_raw text."""
    if dsn_args is None:
        dsn_args = DEFAULT_DSN_ARGS

    sql = "SELECT batch_id, string_agg(source_raw, ' ') FROM pipeline_items_v2 WHERE batch_id IS NOT NULL AND batch_id != '' GROUP BY batch_id"

    result = subprocess.run(
        [psql, *dsn_args, "-t", "-A", "-c", sql],
        capture_output=True, text=True, encoding="utf-8",
        env={**os.environ, "PGPASSWORD": "postgres"},
    )

    if result.returncode != 0:
        print(f"ERROR: psql query failed: {result.stderr}", file=sys.stderr)
        sys.exit(1)

    batches: dict[str, str] = {}
    for line in result.stdout.strip().split("\n"):
        line = line.strip()
        if not line:
            continue
        parts = line.split("|", 1)
        if len(parts) == 2:
            batch_id, text = parts
            batches[batch_id.strip()] = text.strip()

    return batches


# --- Main ---

def main():
    # Step 1: Load enriched termbank
    print(f"Loading enriched termbank from {ENRICHED_TERMBANK_PATH}...")
    with open(ENRICHED_TERMBANK_PATH, encoding="utf-8") as f:
        raw_entries = json.load(f)

    entries = [
        TermEntry(
            term=e["term"],
            description=e.get("description", ""),
            category=e.get("category", ""),
            aliases=e.get("aliases", []),
        )
        for e in raw_entries
        if e.get("description", "").strip()
    ]
    print(f"  Loaded {len(entries)} entries with descriptions")

    # Step 2: Fetch batch texts from DB
    print("Fetching batch texts from PostgreSQL...")
    batches = fetch_batch_texts()
    print(f"  Fetched {len(batches)} batches")

    if not batches:
        print("ERROR: No batches found in pipeline_items", file=sys.stderr)
        sys.exit(1)

    # Step 3: Match RAG hints for each batch
    print("Matching RAG hints per batch...")
    output: dict[str, list[dict]] = {}
    with_hints = 0

    for batch_id, batch_text in batches.items():
        hints = match_rag_hints(entries, batch_text)
        output[batch_id] = hints
        if hints:
            with_hints += 1

    # Step 4: Write output
    with open(OUTPUT_PATH, "w", encoding="utf-8") as f:
        json.dump(output, f, ensure_ascii=False, indent=2)

    total = len(output)
    pct = (with_hints / total * 100) if total > 0 else 0
    print(f"\nBuilt RAG context for {total} batches. Batches with hints: {with_hints}/{total} ({pct:.1f}%)")
    print(f"Output: {OUTPUT_PATH}")


if __name__ == "__main__":
    main()
