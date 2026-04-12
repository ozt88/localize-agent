#!/usr/bin/env python3
"""
Wiki JSON to Clean Markdown Converter
======================================
wiki_pages/*.json (283 files) to clean Markdown files in wiki_markdown/.

Removes navigation, categories, infoboxes, and sidebar content.
Converts wikitext section headings to Markdown headings.
Produces one .md file per wiki page with H1 title.

Usage:
    python convert_wiki_to_markdown.py
"""

import json
import os
import re
import sys
from pathlib import Path

sys.stdout.reconfigure(encoding="utf-8", errors="replace")
sys.stderr.reconfigure(encoding="utf-8", errors="replace")

# Use main repo wiki_pages since worktree may not have them (gitignored)
SCRIPT_DIR = Path(__file__).parent
WIKI_PAGES_DIR = SCRIPT_DIR / "wiki_pages"
OUTPUT_DIR = SCRIPT_DIR / "wiki_markdown"

# Sections to strip (navigation, categories, meta)
STRIP_SECTION_PATTERNS = [
    r"^Categories?:",
    r"^See [Aa]lso",
    r"^Navigation",
    r"^External [Ll]inks?",
    r"^References?$",
]

# Compiled pattern for lines to remove entirely
STRIP_LINE_RE = re.compile("|".join(STRIP_SECTION_PATTERNS))


def safe_filename(title: str) -> str:
    """Convert title to safe filename, replacing special chars with underscores."""
    name = re.sub(r"[<>:\"/\\|?*']", "_", title)
    name = name.strip().strip(".")
    return name[:200] if name else "untitled"


def extract_section_headings(wikitext: str) -> dict[str, int]:
    """Extract == Heading == patterns from wikitext with their levels."""
    headings = {}
    for match in re.finditer(r"^(={2,})\s*(.+?)\s*\1\s*$", wikitext, re.MULTILINE):
        level = len(match.group(1))  # == is 2, === is 3
        title = match.group(2).strip()
        headings[title] = level
    return headings


def clean_plain_text(plain_text: str, wikitext: str, title: str) -> str:
    """Convert plain_text + wikitext info into clean Markdown."""
    lines = plain_text.split("\n")
    headings = extract_section_headings(wikitext)

    output_lines = [f"# {title}", ""]

    skip_rest = False
    for line in lines:
        stripped = line.strip()

        # Skip empty lines (will be re-added as paragraph breaks)
        if not stripped:
            # Add paragraph break if we have content
            if output_lines and output_lines[-1] != "":
                output_lines.append("")
            continue

        # Skip category lines (D-04)
        if STRIP_LINE_RE.search(stripped):
            skip_rest = True
            continue

        # If we hit a strip section, skip everything after
        if skip_rest and stripped.startswith("Category:"):
            continue

        # Reset skip_rest if we see a new section heading
        if stripped.startswith("==") and not stripped.startswith("Category:"):
            skip_rest = False

        # Check if this line is a section heading from wikitext
        # plain_text preserves == Heading == format
        heading_match = re.match(r"^(={2,})\s*(.+?)\s*\1\s*$", stripped)
        if heading_match:
            level = len(heading_match.group(1))
            heading_text = heading_match.group(2).strip()
            # Convert to markdown heading (== -> ##, === -> ###)
            md_heading = "#" * level + " " + heading_text
            if output_lines and output_lines[-1] != "":
                output_lines.append("")
            output_lines.append(md_heading)
            output_lines.append("")
            continue

        # Check if line matches a known heading (without == markers)
        if stripped in headings:
            level = headings[stripped]
            md_heading = "#" * level + " " + stripped
            if output_lines and output_lines[-1] != "":
                output_lines.append("")
            output_lines.append(md_heading)
            output_lines.append("")
            continue

        # Remove infobox remnants (lines starting with | or }})
        if stripped.startswith("|") or stripped.startswith("}}"):
            continue

        # Remove Category: lines
        if stripped.startswith("Category:"):
            continue

        # Normal content line
        output_lines.append(stripped)

    # Clean up trailing empty lines
    while output_lines and output_lines[-1] == "":
        output_lines.pop()

    # Clean up multiple consecutive empty lines
    result = []
    prev_empty = False
    for line in output_lines:
        if line == "":
            if not prev_empty:
                result.append("")
            prev_empty = True
        else:
            result.append(line)
            prev_empty = False

    return "\n".join(result) + "\n"


def main():
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)

    # Check if wiki_pages exists; if not in worktree, try main repo
    wiki_dir = WIKI_PAGES_DIR
    if not wiki_dir.exists() or not list(wiki_dir.glob("*.json")):
        # Try main repo path
        main_repo = Path(__file__).resolve()
        # Walk up to find the repo root with .git
        for parent in main_repo.parents:
            candidate = parent / "projects" / "esoteric-ebb" / "rag" / "wiki_pages"
            if candidate.exists() and list(candidate.glob("*.json")):
                wiki_dir = candidate
                break

    if not wiki_dir.exists():
        print(f"ERROR: wiki_pages directory not found at {WIKI_PAGES_DIR}")
        sys.exit(1)

    json_files = sorted(wiki_dir.glob("*.json"))
    print(f"=== Wiki JSON to Markdown Converter ===")
    print(f"Input:  {wiki_dir} ({len(json_files)} files)")
    print(f"Output: {OUTPUT_DIR}")
    print()

    # D-15: Check JournalTexts.txt
    print("NOTE: JournalTexts.txt is empty per research -- no in-game codex entries")
    print()

    converted = 0
    empty_pages = 0
    errors = 0

    for json_file in json_files:
        try:
            with open(json_file, "r", encoding="utf-8") as f:
                data = json.load(f)
        except (json.JSONDecodeError, OSError) as e:
            print(f"  ERROR reading {json_file.name}: {e}")
            errors += 1
            continue

        title = data.get("title", json_file.stem)
        plain_text = data.get("plain_text", "")
        wikitext = data.get("wikitext", "")

        # Generate safe filename
        md_filename = safe_filename(title) + ".md"
        md_path = OUTPUT_DIR / md_filename

        # Convert to markdown (D-03, D-05: include empty pages too)
        if not plain_text.strip():
            # Empty page -- create file with just the title
            md_content = f"# {title}\n"
            empty_pages += 1
        else:
            md_content = clean_plain_text(plain_text, wikitext, title)

        with open(md_path, "w", encoding="utf-8") as f:
            f.write(md_content)

        converted += 1

    print(f"=== Done ===")
    print(f"Converted {converted} wiki pages to wiki_markdown/")
    print(f"  Empty pages: {empty_pages}")
    print(f"  Errors: {errors}")


if __name__ == "__main__":
    main()
