#!/usr/bin/env python3
"""
Enriched Termbank Builder
=========================
3개 소스를 통합하여 enriched_termbank.json 생성:
  1. esoteric_ebb_lore_termbank.json (wiki lore 254건)
  2. GlossaryTerms.txt (게임 내 용어집 ~800건)
  3. wiki_markdown/ 섹션 인덱스 (Task 1 산출물)

출력: enriched_termbank.json (JSON array, Go []RAGEntry 직접 Unmarshal 가능)

Usage:
    python build_enriched_termbank.py
"""

import csv
import json
import os
import re
import sys
from pathlib import Path

sys.stdout.reconfigure(encoding="utf-8", errors="replace")
sys.stderr.reconfigure(encoding="utf-8", errors="replace")

BASE_DIR = Path(__file__).parent
LORE_TERMBANK_PATH = BASE_DIR / "esoteric_ebb_lore_termbank.json"
GLOSSARY_PATH = (
    BASE_DIR.parent
    / "extract"
    / "1.1.3"
    / "ExportedProject"
    / "Assets"
    / "Resources"
    / "glossaryterms"
    / "GlossaryTerms.txt"
)
WIKI_MARKDOWN_DIR = BASE_DIR / "wiki_markdown"
OUTPUT_PATH = BASE_DIR / "enriched_termbank.json"

MAX_DESC_LENGTH = 300


def truncate_to_two_sentences(text: str) -> str:
    """첫 2문장만 남기고 300자 이내로 자른다."""
    sentences = re.split(r"(?<=[.!?])\s+", text.strip())
    result = " ".join(sentences[:2]).strip()
    if len(result) > MAX_DESC_LENGTH:
        result = result[: MAX_DESC_LENGTH - 3] + "..."
    return result


def extract_term_description(english: str) -> tuple[str, str]:
    """GlossaryTerms의 ENGLISH 컬럼에서 term과 description을 분리한다.
    Go의 extractTermName 로직과 대칭 — 구분자 앞이 term, 뒤가 description."""
    for sep in [" - ", " – "]:
        idx = english.find(sep)
        if idx > 0:
            return english[:idx].strip(), english[idx + len(sep) :].strip()
    return english.strip(), ""


def load_lore_termbank() -> dict[str, dict]:
    """Source 1: 기존 wiki lore termbank (254건)"""
    with open(LORE_TERMBANK_PATH, "r", encoding="utf-8") as f:
        data = json.load(f)

    entries = {}
    for term, entry in data.items():
        desc = entry.get("lore", "")
        if not desc:
            continue
        desc = truncate_to_two_sentences(desc)
        aliases = entry.get("related", [])[:5]
        entries[term] = {
            "term": term,
            "description": desc,
            "category": entry.get("category", "lore"),
            "source": "wiki",
            "aliases": aliases,
        }
    return entries


def load_glossary_terms() -> dict[str, dict]:
    """Source 2: GlossaryTerms.txt (~800건)"""
    entries = {}
    with open(GLOSSARY_PATH, "r", encoding="utf-8") as f:
        reader = csv.reader(f)
        header = next(reader, None)
        if not header:
            return entries

        # Find ENGLISH column index
        english_idx = None
        tags_idx = None
        for i, col in enumerate(header):
            if col.strip().upper() == "ENGLISH":
                english_idx = i
            if col.strip().upper() == "TAGS":
                tags_idx = i

        if english_idx is None:
            print("  WARNING: ENGLISH column not found in GlossaryTerms.txt")
            return entries

        for row in reader:
            if len(row) <= english_idx:
                continue
            english = row[english_idx].strip()
            if not english:
                continue

            term, description = extract_term_description(english)
            if not description:
                continue

            description = truncate_to_two_sentences(description)

            # category from Tags column
            category = "glossary"
            if tags_idx is not None and len(row) > tags_idx:
                tag = row[tags_idx].strip()
                if tag:
                    category = tag.lower()

            entries[term] = {
                "term": term,
                "description": description,
                "category": category,
                "source": "glossary",
                "aliases": [],
            }

    return entries


def load_wiki_sections(existing_terms: set[str]) -> dict[str, dict]:
    """Source 3: wiki_markdown/ 섹션 인덱스 — lore termbank에 없는 페이지만 추가"""
    entries = {}
    if not WIKI_MARKDOWN_DIR.exists():
        return entries

    for md_file in sorted(WIKI_MARKDOWN_DIR.iterdir()):
        if not md_file.suffix == ".md":
            continue

        # term = filename without extension, restore underscores to spaces
        term = md_file.stem.replace("_", " ").strip()

        # 이미 lore에 있으면 skip
        if term in existing_terms:
            continue

        try:
            text = md_file.read_text(encoding="utf-8")
        except Exception:
            continue

        if not text.strip():
            continue

        # H1 아래 첫 2문장 추출
        lines = text.strip().split("\n")
        content_lines = []
        past_h1 = False
        for line in lines:
            if line.startswith("# "):
                past_h1 = True
                continue
            if past_h1:
                stripped = line.strip()
                if not stripped:
                    if content_lines:
                        break
                    continue
                if stripped.startswith("## "):
                    if content_lines:
                        break
                    continue
                content_lines.append(stripped)

        if not content_lines:
            continue

        paragraph = " ".join(content_lines)
        description = truncate_to_two_sentences(paragraph)
        if not description or len(description) < 10:
            continue

        entries[term] = {
            "term": term,
            "description": description,
            "category": "wiki_section",
            "source": "wiki_section",
            "aliases": [],
        }

    return entries


def main():
    print("=== Enriched Termbank Builder ===\n")

    # Source 1: Wiki lore
    wiki_entries = load_lore_termbank()
    print(f"Source 1 (wiki lore): {len(wiki_entries)} entries")

    # Source 2: GlossaryTerms
    glossary_entries = load_glossary_terms()
    print(f"Source 2 (glossary): {len(glossary_entries)} entries")

    # Source 3: Wiki sections (only terms not in lore)
    existing_terms = set(wiki_entries.keys())
    section_entries = load_wiki_sections(existing_terms)
    print(f"Source 3 (wiki sections): {len(section_entries)} entries")

    # Merge: wiki lore > glossary > wiki_section
    merged = {}

    # Start with glossary (lowest priority)
    for term, entry in glossary_entries.items():
        merged[term] = entry

    # Wiki sections override glossary
    for term, entry in section_entries.items():
        if term not in merged:
            merged[term] = entry

    # Wiki lore has highest priority
    for term, entry in wiki_entries.items():
        if term in merged:
            # Merge aliases from glossary if both exist
            existing = merged[term]
            if existing.get("aliases"):
                combined_aliases = list(
                    set(entry.get("aliases", []) + existing["aliases"])
                )[:5]
                entry["aliases"] = combined_aliases
        merged[term] = entry

    # Convert to JSON array (for Go []RAGEntry Unmarshal)
    result = sorted(merged.values(), key=lambda x: x["term"])

    # Validate
    for entry in result:
        assert len(entry["description"]) <= MAX_DESC_LENGTH, (
            f"{entry['term']}: description too long ({len(entry['description'])})"
        )
        assert entry["source"] in ("wiki", "glossary", "wiki_section"), (
            f"{entry['term']}: invalid source {entry['source']}"
        )

    # Write output
    with open(OUTPUT_PATH, "w", encoding="utf-8") as f:
        json.dump(result, f, ensure_ascii=False, indent=2)

    # Stats
    sources = {}
    for e in result:
        s = e["source"]
        sources[s] = sources.get(s, 0) + 1

    print(f"\nBuilt enriched termbank: {len(result)} entries", end="")
    parts = [f"{k}: {v}" for k, v in sorted(sources.items())]
    print(f" ({', '.join(parts)})")
    print(f"Output: {OUTPUT_PATH}")


if __name__ == "__main__":
    main()
