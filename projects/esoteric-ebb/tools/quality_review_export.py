#!/usr/bin/env python3
"""
Generate a translation quality review TSV comparing v2 and v1 Korean translations.

Reads v2 translations.json (v3 sidecar format) and v1 translations.json
(v2 sidecar format from artifacts), matches entries by exact source text,
and outputs a side-by-side TSV for manual review.

Usage:
    python quality_review_export.py

Output:
    projects/esoteric-ebb/output/v2/quality_review.tsv
"""

import json
import sys
from pathlib import Path

SCRIPT_DIR = Path(__file__).parent
PROJECT_DIR = SCRIPT_DIR.parent

V2_PATH = PROJECT_DIR / "output" / "v2" / "translations.json"
V1_PATH = (
    PROJECT_DIR
    / "patch"
    / "output"
    / "korean_patch_build_v1_final"
    / "artifacts"
    / "translations.json"
)
OUTPUT_PATH = PROJECT_DIR / "output" / "v2" / "quality_review.tsv"

MAX_TEXT_LEN = 200

# v1 statuses considered valid translations
V1_VALID_STATUSES = {"translated", "reviewed"}


def truncate(text: str, max_len: int = MAX_TEXT_LEN) -> str:
    """Truncate text and replace internal newlines with a visible marker."""
    if not text:
        return ""
    # Replace newlines with visible marker for TSV readability
    text = text.replace("\n", " ↵ ").replace("\r", "")
    if len(text) > max_len:
        return text[:max_len] + "…"
    return text


def load_v2(path: Path) -> list[dict]:
    """Load v2 translations.json (format: esoteric-ebb-sidecar.v3)."""
    print(f"Loading v2: {path}", flush=True)
    with open(path, encoding="utf-8") as f:
        data = json.load(f)

    fmt = data.get("format", "")
    entries = data.get("entries", [])
    print(f"  format={fmt!r}, entries={len(entries)}", flush=True)
    return entries


def load_v1(path: Path) -> dict[str, str]:
    """
    Load v1 translations.json (format: esoteric-ebb-sidecar.v2).

    Returns a dict mapping source_text -> target_text for valid-status entries.
    When multiple entries share the same source, last one wins (they're usually
    the same translation anyway).
    """
    print(f"Loading v1: {path}", flush=True)
    with open(path, encoding="utf-8") as f:
        data = json.load(f)

    fmt = data.get("format", "")
    entries = data.get("entries", [])
    print(f"  format={fmt!r}, entries={len(entries)}", flush=True)

    lookup: dict[str, str] = {}
    skipped = 0
    for entry in entries:
        # Support both field name variants
        source = entry.get("source") or entry.get("source_text", "")
        target = entry.get("target") or entry.get("target_text", "")
        status = entry.get("status", "")

        if not source:
            skipped += 1
            continue

        # Only include entries with a valid translation status
        if status and status not in V1_VALID_STATUSES:
            skipped += 1
            continue

        if target:
            lookup[source] = target

    print(f"  v1 lookup built: {len(lookup)} entries (skipped {skipped})", flush=True)
    return lookup


def compute_diff(v2_target: str, v1_target: str) -> str:
    """Compute diff label for the two translation sides."""
    has_v2 = bool(v2_target)
    has_v1 = bool(v1_target)

    if has_v2 and not has_v1:
        return "v2_only"
    if has_v1 and not has_v2:
        return "v1_only"
    if has_v2 and has_v1:
        if v2_target.strip() == v1_target.strip():
            return "both_same"
        return "both_diff"
    # Neither side has a target — caller should have filtered this out
    return "both_empty"


def build_rows(v2_entries: list[dict], v1_lookup: dict[str, str]) -> list[dict]:
    """
    Build review rows by matching v2 entries against v1 lookup.

    Filters:
    - Skip entries whose source contains newlines (block-level, not useful for review)
    - Skip entries where both sides have empty targets
    """
    rows = []
    skipped_newline = 0
    skipped_empty = 0

    for entry in v2_entries:
        source = entry.get("source", "")
        v2_target = entry.get("target", "")
        source_file = entry.get("source_file", "")
        text_role = entry.get("text_role", "")
        entry_id = entry.get("id", "")

        # Skip block-level entries (source contains newlines)
        if "\n" in source:
            skipped_newline += 1
            continue

        # Look up v1 translation by exact source match
        v1_target = v1_lookup.get(source, "")

        # Skip entries where both sides have no translation
        if not v2_target and not v1_target:
            skipped_empty += 1
            continue

        diff = compute_diff(v2_target, v1_target)

        rows.append(
            {
                "source_file": source_file,
                "source": source,
                "v2_target": v2_target,
                "v1_target": v1_target,
                "diff": diff,
                "text_role": text_role,
                "id": entry_id,
            }
        )

    print(
        f"  rows built: {len(rows)} "
        f"(skipped newline={skipped_newline}, empty={skipped_empty})",
        flush=True,
    )
    return rows


def write_tsv(rows: list[dict], output_path: Path) -> None:
    """Write rows to TSV with UTF-8 BOM for Excel compatibility."""
    output_path.parent.mkdir(parents=True, exist_ok=True)

    # Sort by source_file then by id
    rows.sort(key=lambda r: (r["source_file"], r["id"]))

    columns = ["source_file", "source", "v2_target", "v1_target", "diff", "text_role", "id"]

    # UTF-8 BOM so Excel opens without garbled Korean text
    with open(output_path, "w", encoding="utf-8-sig", newline="") as f:
        # Header
        f.write("\t".join(columns) + "\n")

        for row in rows:
            fields = [
                row["source_file"],
                truncate(row["source"]),
                truncate(row["v2_target"]),
                truncate(row["v1_target"]),
                row["diff"],
                row["text_role"],
                row["id"],
            ]
            # Escape any remaining tabs in field values
            escaped = [field.replace("\t", " ") for field in fields]
            f.write("\t".join(escaped) + "\n")

    print(f"Written: {output_path}", flush=True)
    print(f"  Total rows: {len(rows)}", flush=True)


def print_summary(rows: list[dict]) -> None:
    """Print a brief diff-label breakdown."""
    from collections import Counter

    diff_counts = Counter(r["diff"] for r in rows)
    print("\n--- Diff summary ---")
    for label in ("both_diff", "v2_only", "v1_only", "both_same"):
        print(f"  {label}: {diff_counts.get(label, 0)}")

    role_counts = Counter(r["text_role"] for r in rows)
    print("\n--- text_role breakdown (top 10) ---")
    for role, count in role_counts.most_common(10):
        print(f"  {role!r}: {count}")


def main() -> int:
    # Validate input paths
    if not V2_PATH.exists():
        print(f"ERROR: v2 translations not found: {V2_PATH}", file=sys.stderr)
        return 1
    if not V1_PATH.exists():
        print(f"ERROR: v1 translations not found: {V1_PATH}", file=sys.stderr)
        return 1

    v2_entries = load_v2(V2_PATH)
    v1_lookup = load_v1(V1_PATH)

    rows = build_rows(v2_entries, v1_lookup)
    write_tsv(rows, OUTPUT_PATH)
    print_summary(rows)

    return 0


if __name__ == "__main__":
    sys.exit(main())
