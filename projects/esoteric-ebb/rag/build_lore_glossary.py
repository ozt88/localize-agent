"""
Esoteric Ebb Lore Glossary Builder
===================================
스크랩된 위키 데이터에서 로어 정보를 추출하여
번역 파이프라인용 lore glossary를 생성합니다.

출력:
  - esoteric_ebb_lore_termbank.json  (Lore_Termbank 형식)
  - esoteric_ebb_lore_summary.json   (통계/요약)

Usage:
    python build_lore_glossary.py
"""

import json
import re
import sys
from pathlib import Path

sys.stdout.reconfigure(encoding="utf-8", errors="replace")
sys.stderr.reconfigure(encoding="utf-8", errors="replace")

WIKI_PAGES_DIR = Path(__file__).parent / "wiki_pages"
INDEX_PATH = Path(__file__).parent / "wiki_index.json"
OUTPUT_DIR = Path(__file__).parent
EXISTING_GLOSSARY = Path(__file__).parent.parent.parent / "workflow" / "context" / "universal_glossary.json"

# 위키 카테고리 → lore 카테고리 매핑
CATEGORY_MAP = {
    "Characters": "character",
    "Humans": "character",
    "Gnomes": "character",
    "Historical_Figures": "character",
    "Races": "race",
    "Items": "item",
    "Equipment": "item",
    "Helmets": "item",
    "Weapons": "item",
    "Armor": "item",
    "Consumables": "item",
    "Key_Items": "item",
    "Mechanics": "mechanic",
    "Spells": "spell",
    "Cantrips": "spell",
    "Feats": "feat",
    "Demo_pages": None,  # 무시
    "Stubs": None,
    "Pages_with_spoilers": None,
    "Pages_with_DRUID_infoboxes": None,
    "Pages_using_EmbedVideo": None,
    "Quests": "quest",
    "Locations": "location",
    "Factions": "faction",
    "Gods": "deity",
    "Literature": "literature",
    "Political_Parties": "faction",
}

# 제외할 페이지 (메타 페이지)
SKIP_TITLES = {
    "Main Page",
    "Esoteric Ebb Wiki",
    "Esoteric Ebb Wiki/about",
    "Esoteric Ebb Wiki/contribute",
    "Esoteric Ebb Wiki/external",
    "Esoteric Ebb Wiki/pages",
    "Esoteric Ebb Wiki/welcome",
}

# 최소 텍스트 길이 (너무 짧은 페이지는 유용한 로어가 없음)
MIN_CHARS = 30


def load_index() -> list[dict]:
    with open(INDEX_PATH, "r", encoding="utf-8") as f:
        return json.load(f)


def load_page(filename: str) -> dict | None:
    path = WIKI_PAGES_DIR / filename
    if not path.exists():
        return None
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def load_existing_glossary() -> dict:
    if not EXISTING_GLOSSARY.exists():
        return {}
    with open(EXISTING_GLOSSARY, "r", encoding="utf-8") as f:
        return json.load(f)


def extract_first_sentence(plain_text: str) -> str:
    """첫 번째 의미 있는 문장을 추출합니다."""
    # 인포박스 잔여물 제거
    text = re.sub(r'^\|[^\n]*\n', '', plain_text, flags=re.MULTILINE)
    text = re.sub(r'^}}.*?\n', '', text, flags=re.MULTILINE)
    # 잔여 위키 마크업 제거
    text = re.sub(r'\[\[(?:File|Image):[^\]]*\]\]', '', text)
    text = re.sub(r'\[\[(?:[^|\]]*\|)?([^\]]+)\]\]', r'\1', text)
    text = re.sub(r'\{\{[^}]*\}\}', '', text)
    text = re.sub(r'<[^>]+>', '', text)
    text = re.sub(r"'{2,3}", '', text)
    # 잔여 ]] 나 ) 등 제거
    text = re.sub(r'^[\]\)\|\}\s]+', '', text)

    # 빈 줄과 섹션 헤더 전까지의 첫 문단
    lines = []
    for line in text.strip().split('\n'):
        line = line.strip()
        if not line:
            if lines:
                break
            continue
        if line.startswith('==') or line.startswith('= '):
            if lines:
                break
            continue  # 섹션 헤더는 스킵하고 다음 줄로
        if line.startswith('|') or line.startswith('}}'):
            continue
        # 너무 짧은 잔여 마크업 줄 스킵
        cleaned = re.sub(r'[\[\]\(\)\{\}\|]', '', line).strip()
        if len(cleaned) < 5:
            continue
        lines.append(line)

    paragraph = ' '.join(lines).strip()
    if not paragraph:
        return ""

    # 첫 2문장 추출 (로어 설명으로 충분)
    sentences = re.split(r'(?<=[.!?])\s+', paragraph)
    result = ' '.join(sentences[:2]).strip()

    # 너무 길면 자르기
    if len(result) > 300:
        result = result[:297] + "..."

    return result


def extract_aliases_from_wikitext(wikitext: str, title: str) -> list[str]:
    """위키텍스트에서 별칭을 추출합니다."""
    aliases = set()

    # 인포박스에서 Names 필드 추출
    names_match = re.search(r'\|Names?=(.+?)(?:\n|\|)', wikitext)
    if names_match:
        names_raw = names_match.group(1)
        # 위키 마크업 제거
        names_raw = re.sub(r"'{2,3}", '', names_raw)
        names_raw = re.sub(r'\[\[(?:[^|\]]*\|)?([^\]]+)\]\]', r'\1', names_raw)
        names_raw = re.sub(r'\{\{[^}]*\}\}', '', names_raw)
        for name in re.split(r',\s*', names_raw):
            name = name.strip()
            # 위키 마크업 잔여물 필터링
            if re.search(r'\{\{|\}\}|\[\[|\]\]|<|>', name):
                continue
            if name and name != title and len(name) > 1:
                aliases.add(name)

    # 리다이렉트 패턴에서 별칭 추출
    redirect_match = re.search(r'#REDIRECT\s*\[\[([^\]]+)\]\]', wikitext, re.IGNORECASE)
    if redirect_match:
        aliases.add(redirect_match.group(1).strip())

    return sorted(aliases)


def infer_category(wiki_categories: list[str]) -> str:
    """위키 카테고리에서 lore 카테고리를 추론합니다."""
    for cat in wiki_categories:
        mapped = CATEGORY_MAP.get(cat)
        if mapped:
            return mapped

    # 카테고리가 매핑에 없는 경우, 카테고리 이름 자체를 소문자로
    for cat in wiki_categories:
        if CATEGORY_MAP.get(cat) is None and cat in CATEGORY_MAP:
            continue  # 명시적 None (무시 대상)
        return cat.lower().replace('_', ' ')

    return "lore"


def find_related_terms(plain_text: str, all_titles: set[str], own_title: str) -> list[str]:
    """텍스트에서 다른 위키 페이지 제목과 일치하는 관련 용어를 찾습니다."""
    related = set()
    for title in all_titles:
        if title == own_title:
            continue
        if len(title) < 3:
            continue
        # 단어 경계 매칭
        pattern = r'(?<![A-Za-z])' + re.escape(title) + r'(?![A-Za-z])'
        if re.search(pattern, plain_text, re.IGNORECASE):
            related.add(title)
    # 너무 많으면 상위 10개만
    return sorted(related)[:10]


def main():
    print("=== Esoteric Ebb Lore Glossary Builder ===\n")

    # 1. 인덱스 로드
    index = load_index()
    print(f"Loaded {len(index)} wiki pages from index.")

    # 2. 기존 glossary 로드 (중복 체크용)
    existing = load_existing_glossary()
    existing_terms = set()
    # Lore_Proper_Nouns에서 기존 용어 수집
    for section_key in ["Lore_Proper_Nouns", "System_Stats", "Combat_Mechanics"]:
        if section_key in existing:
            existing_terms.update(existing[section_key].keys())
    # Lore_Termbank에서도
    if "Lore_Termbank" in existing:
        existing_terms.update(existing["Lore_Termbank"].keys())
    print(f"Found {len(existing_terms)} existing glossary terms.")

    # 전체 타이틀 셋 (관련 용어 추출용)
    all_titles = {entry["title"] for entry in index}

    # 3. 각 페이지에서 로어 추출
    lore_termbank = {}
    stats = {"total": 0, "skipped_meta": 0, "skipped_short": 0, "new": 0, "existing_enriched": 0}

    for entry in index:
        title = entry["title"]
        stats["total"] += 1

        # 메타 페이지 스킵
        if title in SKIP_TITLES:
            stats["skipped_meta"] += 1
            continue

        # 짧은 페이지 스킵
        if entry["chars"] < MIN_CHARS:
            stats["skipped_short"] += 1
            continue

        page = load_page(entry["file"])
        if not page:
            continue

        plain_text = page.get("plain_text", "")
        wikitext = page.get("wikitext", "")
        wiki_categories = entry.get("categories", [])

        # 로어 설명 추출
        lore_desc = extract_first_sentence(plain_text)
        if not lore_desc:
            stats["skipped_short"] += 1
            continue

        # 카테고리 추론
        category = infer_category(wiki_categories)

        # 별칭 추출
        aliases = extract_aliases_from_wikitext(wikitext, title)

        # 관련 용어
        related = find_related_terms(plain_text, all_titles, title)

        # 엔트리 생성
        lore_entry = {
            "lore": lore_desc,
            "category": category,
            "wiki_url": page.get("url", ""),
        }

        if aliases:
            lore_entry["aliases"] = aliases
        if related:
            lore_entry["related"] = related

        # 기존 glossary에 있는 용어인지 체크
        if title in existing_terms:
            lore_entry["status"] = "existing_term"
            stats["existing_enriched"] += 1
        else:
            lore_entry["status"] = "new"
            stats["new"] += 1

        lore_termbank[title] = lore_entry

    # 4. 카테고리별 정렬
    sorted_termbank = dict(sorted(lore_termbank.items(), key=lambda x: (x[1]["category"], x[0])))

    # 5. 출력
    output_path = OUTPUT_DIR / "esoteric_ebb_lore_termbank.json"
    with open(output_path, "w", encoding="utf-8") as f:
        json.dump(sorted_termbank, f, ensure_ascii=False, indent=2)
    print(f"\nLore termbank saved: {output_path}")
    print(f"  Total entries: {len(sorted_termbank)}")

    # 6. 카테고리별 통계
    cat_counts = {}
    for entry in sorted_termbank.values():
        cat = entry["category"]
        cat_counts[cat] = cat_counts.get(cat, 0) + 1

    summary = {
        "stats": stats,
        "categories": dict(sorted(cat_counts.items(), key=lambda x: -x[1])),
        "total_entries": len(sorted_termbank),
        "sample_entries": {k: v for k, v in list(sorted_termbank.items())[:5]},
    }

    summary_path = OUTPUT_DIR / "esoteric_ebb_lore_summary.json"
    with open(summary_path, "w", encoding="utf-8") as f:
        json.dump(summary, f, ensure_ascii=False, indent=2)

    print(f"\n=== Summary ===")
    print(f"  Pages processed: {stats['total']}")
    print(f"  Skipped (meta):  {stats['skipped_meta']}")
    print(f"  Skipped (short): {stats['skipped_short']}")
    print(f"  New terms:       {stats['new']}")
    print(f"  Enriched terms:  {stats['existing_enriched']}")
    print(f"\nCategories:")
    for cat, count in sorted(cat_counts.items(), key=lambda x: -x[1]):
        print(f"  {cat:20s} {count:4d}")


if __name__ == "__main__":
    main()
