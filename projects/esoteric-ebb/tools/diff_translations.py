#!/usr/bin/env python3
"""Compare two translations.json files and output changed entries.

Responsibilities:
- Load two translations.json files (before/after)
- Compute per-entry diff by source string
- Output summary stats and optionally write diff JSON

Usage:
    python diff_translations.py <before.json> <after.json> [--out diff.json] [--summary]
"""

import json
import sys
import argparse


def diff_translations(before_path, after_path):
    with open(before_path, encoding="utf-8") as f:
        before_entries = json.load(f)["entries"]
    with open(after_path, encoding="utf-8") as f:
        after_entries = json.load(f)["entries"]

    before_map = {e["source"]: e["target"] for e in before_entries}
    after_map = {e["source"]: e["target"] for e in after_entries}

    all_sources = set(before_map) | set(after_map)
    changes = []
    unchanged = 0
    added = 0
    removed = 0

    for src in sorted(all_sources):
        b = before_map.get(src)
        a = after_map.get(src)
        if b == a:
            unchanged += 1
        elif b is None:
            added += 1
            changes.append({"source": src, "before": None, "after": a, "type": "added"})
        elif a is None:
            removed += 1
            changes.append({"source": src, "before": b, "after": None, "type": "removed"})
        else:
            changes.append({"source": src, "before": b, "after": a, "type": "changed"})

    changed = len([c for c in changes if c["type"] == "changed"])
    stats = {
        "total": len(all_sources),
        "changed": changed,
        "added": added,
        "removed": removed,
        "unchanged": unchanged,
    }
    return changes, stats


def main():
    parser = argparse.ArgumentParser(description="Diff translations.json files")
    parser.add_argument("before", help="Before translations.json")
    parser.add_argument("after", help="After translations.json")
    parser.add_argument("--out", default=None, help="Output diff JSON path")
    parser.add_argument("--summary", action="store_true", help="Print summary only")
    args = parser.parse_args()

    changes, stats = diff_translations(args.before, args.after)

    print(f"Total entries: {stats['total']}")
    print(f"Changed:       {stats['changed']}")
    print(f"Added:         {stats['added']}")
    print(f"Removed:       {stats['removed']}")
    print(f"Unchanged:     {stats['unchanged']}")

    if args.out:
        with open(args.out, "w", encoding="utf-8") as f:
            json.dump({"stats": stats, "changes": changes}, f, ensure_ascii=False, indent=2)
        print(f"Diff written to {args.out}")

    if not args.summary and not args.out:
        for c in changes[:20]:
            print(f"\n[{c['type']}] {c['source'][:60]}")
            if c["before"]:
                print(f"  BEFORE: {c['before'][:80]}")
            if c["after"]:
                print(f"  AFTER:  {c['after'][:80]}")
        if len(changes) > 20:
            print(f"\n... and {len(changes) - 20} more changes")


if __name__ == "__main__":
    main()
