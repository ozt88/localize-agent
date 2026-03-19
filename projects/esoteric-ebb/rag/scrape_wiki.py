"""
Esoteric Ebb Wiki Scraper
=========================
MediaWiki API를 사용하여 esotericebb.wiki.gg 전체 페이지를 스크랩합니다.
결과는 wiki_pages/ 디렉터리에 JSON 파일로 저장됩니다.

Usage:
    python scrape_wiki.py
"""

import json
import os
import re
import sys
import time
import urllib.request
import urllib.parse
from pathlib import Path

# Windows cp949 인코딩 문제 방지
sys.stdout.reconfigure(encoding="utf-8", errors="replace")
sys.stderr.reconfigure(encoding="utf-8", errors="replace")

WIKI_API = "https://esotericebb.wiki.gg/api.php"
OUTPUT_DIR = Path(__file__).parent / "wiki_pages"
RATE_LIMIT_SEC = 0.5  # 서버 부하 방지


def api_request(params: dict) -> dict:
    params["format"] = "json"
    url = f"{WIKI_API}?{urllib.parse.urlencode(params)}"
    req = urllib.request.Request(url, headers={"User-Agent": "localize-agent-wiki-scraper/1.0"})
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.loads(resp.read().decode("utf-8"))


def get_all_pages() -> list[dict]:
    """모든 메인 네임스페이스(ns=0) 페이지 목록을 가져옵니다."""
    pages = []
    params = {"action": "query", "list": "allpages", "aplimit": "50", "apnamespace": "0"}
    while True:
        data = api_request(params)
        pages.extend(data["query"]["allpages"])
        if "continue" in data:
            params["apcontinue"] = data["continue"]["apcontinue"]
        else:
            break
        time.sleep(RATE_LIMIT_SEC)
    return pages


def get_page_content(title: str) -> dict | None:
    """페이지의 plain text 콘텐츠와 카테고리를 가져옵니다."""
    params = {
        "action": "parse",
        "page": title,
        "prop": "wikitext|categories",
        "disabletoc": "true",
    }
    try:
        data = api_request(params)
    except Exception as e:
        print(f"  ERROR fetching '{title}': {e}")
        return None

    parse = data.get("parse", {})
    wikitext = parse.get("wikitext", {}).get("*", "")
    categories = [c["*"] for c in parse.get("categories", [])]

    # wikitext -> 간단한 plain text 변환
    plain = wikitext_to_plain(wikitext)

    return {
        "title": title,
        "pageid": parse.get("pageid"),
        "url": f"https://esotericebb.wiki.gg/wiki/{urllib.parse.quote(title.replace(' ', '_'))}",
        "categories": categories,
        "wikitext": wikitext,
        "plain_text": plain,
    }


def wikitext_to_plain(text: str) -> str:
    """위키텍스트를 읽기 쉬운 plain text로 변환합니다."""
    # 이미지/파일 링크 제거
    text = re.sub(r'\[\[(?:File|Image):[^\]]*\]\]', '', text)
    # 내부 링크: [[link|display]] -> display, [[link]] -> link
    text = re.sub(r'\[\[(?:[^|\]]*\|)?([^\]]+)\]\]', r'\1', text)
    # 외부 링크: [url text] -> text
    text = re.sub(r'\[https?://\S+\s+([^\]]+)\]', r'\1', text)
    # 외부 링크: [url] -> (제거)
    text = re.sub(r'\[https?://\S+\]', '', text)
    # HTML 태그 제거
    text = re.sub(r'<[^>]+>', '', text)
    # 템플릿 간단 처리: {{template|arg}} -> arg (간단한 케이스만)
    text = re.sub(r'\{\{[^}]*\}\}', '', text)
    # 볼드/이탤릭
    text = re.sub(r"'{2,3}", '', text)
    # 연속 빈 줄 정리
    text = re.sub(r'\n{3,}', '\n\n', text)
    return text.strip()


def safe_filename(title: str) -> str:
    """파일명에 사용할 수 없는 문자를 치환합니다."""
    name = re.sub(r'[<>:"/\\|?*]', '_', title)
    return name[:200]  # 파일명 길이 제한


def main():
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)

    print("=== Esoteric Ebb Wiki Scraper ===")
    print(f"Output: {OUTPUT_DIR}")
    print()

    # 1. 전체 페이지 목록
    print("Fetching page list...")
    pages = get_all_pages()
    print(f"Found {len(pages)} pages.")
    print()

    # 2. 각 페이지 콘텐츠 스크랩
    success = 0
    errors = 0
    for i, page in enumerate(pages, 1):
        title = page["title"]
        print(f"[{i}/{len(pages)}] {title}...", end=" ", flush=True)

        content = get_page_content(title)
        if content is None:
            errors += 1
            continue

        # 빈 페이지 스킵
        if not content["plain_text"].strip():
            print("(empty, skipped)")
            continue

        # JSON 저장
        filename = safe_filename(title) + ".json"
        filepath = OUTPUT_DIR / filename
        with open(filepath, "w", encoding="utf-8") as f:
            json.dump(content, f, ensure_ascii=False, indent=2)

        print(f"OK ({len(content['plain_text'])} chars)")
        success += 1
        time.sleep(RATE_LIMIT_SEC)

    print()
    print(f"=== Done: {success} saved, {errors} errors, {len(pages)} total ===")

    # 3. 인덱스 파일 생성
    index = []
    for fp in sorted(OUTPUT_DIR.glob("*.json")):
        with open(fp, "r", encoding="utf-8") as f:
            data = json.load(f)
        index.append({
            "title": data["title"],
            "file": fp.name,
            "categories": data.get("categories", []),
            "chars": len(data.get("plain_text", "")),
        })

    index_path = OUTPUT_DIR.parent / "wiki_index.json"
    with open(index_path, "w", encoding="utf-8") as f:
        json.dump(index, f, ensure_ascii=False, indent=2)
    print(f"Index saved: {index_path} ({len(index)} entries)")


if __name__ == "__main__":
    main()
