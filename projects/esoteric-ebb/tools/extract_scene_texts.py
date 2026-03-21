#!/usr/bin/env python3
"""
Extract translatable m_text values from Unity Scene files (.unity)
and produce an overlay pipeline input package for missing items.

Usage:
    python extract_scene_texts.py [--check-db] [--output scene_overlay_package.json]

Steps:
    1. Parse all .unity scene files for m_text fields
    2. Clean/deduplicate text values
    3. Filter out non-translatable content (numbers, credits, placeholders)
    4. Optionally cross-reference against PostgreSQL to find truly missing items
    5. Output a JSON package ready for the overlay translation pipeline
"""

import argparse
import hashlib
import json
import os
import re
import subprocess
import sys

EXTRACT_DIR = os.path.join(
    os.path.dirname(__file__), "..", "extract", "ExportedProject", "Assets", "Scenes"
)
OUTPUT_DIR = os.path.join(os.path.dirname(__file__), "..", "source")

# Patterns that indicate non-translatable content
SKIP_PATTERNS = [
    re.compile(r"^[\d\s\.\-\+\/:,|]+$"),                    # pure numbers/punctuation
    re.compile(r"^[A-Z]{2,3}:\s*[\d\+\-]"),                  # stat modifiers: "MOD: +1"
    re.compile(r"^AS:\s*\d"),                                  # ability score refs
    re.compile(r"^\+\d+\s+\w+\n?$"),                          # "+0 Athletics"
    re.compile(r"^m_is\w+:"),                                  # Unity metadata leak
    re.compile(r"^\d{2,4}[-/]\d{2}[-/]\d{2}"),               # dates like 2024-01-01
    re.compile(r"^v\d+\.\d+"),                                 # version strings
    re.compile(r"^FontNameHere"),                               # placeholder
    re.compile(r"^[A-Z][a-z]+ [A-Z][a-z]+\n[A-Z]"),          # name lists (credits)
    re.compile(r"^\.\.\.\n"),                                   # ellipsis fillers
    re.compile(r"^©\s*\d{4}"),                                 # copyright notices
]

# Names that are credits (not translatable)
CREDIT_NAMES = {
    "Anders Bach", "Brian Batz", "Kristian Paulsen", "Angel Marcloid",
    "Isac Jonsson", "Jonathan Nilsson", "Olof Aldsjö", "Paulina Wärn",
    "Alva Wirsén Holm", "Emma Bengtsing", "Lovisa Malinen", "Mayowa Gidi",
    "Oscar Westberg", "Tobias Löf Melker", "Jacob Haubjerg", "Mikkel Grevsen",
    "Peter Kohlmetz Møller", "Phil Jamesson", "Jonna Träff",
    "Lars Bech Pilgaard", "Rune Risager",
}

# Categories of text for classification
def classify_text(text: str, scene_file: str) -> str:
    """Classify text into a text_role category."""
    lower = text.lower()
    # UI elements
    ui_keywords = [
        "continue", "save", "load", "cancel", "exit", "return", "reset",
        "settings", "options", "credits", "new game", "skip", "finish",
        "delete", "overwrite", "fullscreen", "resolution", "volume",
        "display", "controls", "language", "quality", "colorblind",
    ]
    for kw in ui_keywords:
        if lower.strip().startswith(kw) or lower.strip() == kw:
            return "ui_label"

    # Character creation / stat descriptions
    if any(w in lower for w in ["cleric", "ability score", "spell", "cantrip",
                                  "proficien", "hit point", "hit dice",
                                  "level up", "feat", "experience"]):
        return "system_text"

    # Flavor/lore text
    if len(text) > 100:
        return "description"

    # Short labels
    if len(text) < 30 and not any(c in text for c in ".!?"):
        return "ui_label"

    return "flavor_text"


def extract_m_text_from_scene(scene_path: str) -> list[dict]:
    """Extract all m_text values from a Unity .unity scene file."""
    results = []
    scene_name = os.path.splitext(os.path.basename(scene_path))[0]

    with open(scene_path, "r", encoding="utf-8", errors="ignore") as f:
        content = f.read()

    # Match m_text fields in YAML-like Unity scene format
    # Can be single-line or multi-line
    pattern = re.compile(
        r"m_text:\s*(?:'((?:[^'\\]|\\'|'')*)'|\"((?:[^\"\\]|\\.)*)\"|(.*?))\s*$",
        re.MULTILINE,
    )

    for match in pattern.finditer(content):
        text = match.group(1) or match.group(2) or match.group(3) or ""
        # Unescape Unity YAML quotes
        text = text.replace("''", "'").replace('\\"', '"')
        # Handle \n and \r
        text = text.replace("\\n", "\n").replace("\\r", "\r")
        text = text.strip()

        if text:
            results.append({
                "raw_text": text,
                "scene_file": scene_name,
            })

    return results


def strip_rich_text_tags(text: str) -> str:
    """Remove TMP rich text tags for comparison purposes."""
    # Remove XML-style tags
    cleaned = re.sub(r"<[^>]*>", "", text)
    # Remove TMP markup like [[E0]], [[/E0]]
    cleaned = re.sub(r"\[\[[^\]]*\]\]", "", cleaned)
    # Remove noparse tags content
    cleaned = re.sub(r"<noparse>.*?</noparse>", "", cleaned)
    return cleaned.strip()


def is_translatable(text: str) -> bool:
    """Determine if text should be translated."""
    stripped = strip_rich_text_tags(text)

    # Too short
    if len(stripped) < 3:
        return False

    # Must contain at least some alphabetic chars
    if not re.search(r"[a-zA-Z]{2,}", stripped):
        return False

    # Skip patterns
    for pat in SKIP_PATTERNS:
        if pat.search(stripped):
            return False

    # Skip pure credit names
    first_line = stripped.split("\n")[0].strip()
    if first_line in CREDIT_NAMES:
        return False

    # Skip if it's a long credits block (multiple names with newlines)
    lines = [l.strip() for l in stripped.split("\n") if l.strip()]
    if len(lines) > 3:
        name_pattern = re.compile(r"^[A-Z][a-zéèêëàáâãäåæ]+ [A-Z][a-zéèêëàáâãäåæ]+")
        name_count = sum(1 for l in lines if name_pattern.match(l))
        if name_count > len(lines) * 0.6:
            return False

    return True


def generate_id(text: str, scene: str) -> str:
    """Generate a stable overlay ID for a scene text item."""
    h = hashlib.sha256(f"{scene}:{text}".encode("utf-8")).hexdigest()[:12]
    return f"ovl-scene-{scene.lower().replace(' ', '_')}-{h}"


def check_db_existence(items: list[dict], dsn: str) -> list[dict]:
    """Check which items already exist in PostgreSQL."""
    psql = "C:/Program Files/PostgreSQL/17/bin/psql.exe"

    # Get all existing en texts from DB
    result = subprocess.run(
        [psql, dsn, "-t", "-A", "-c",
         "SELECT pack_json->>'en' FROM items"],
        capture_output=True, text=True, encoding="utf-8", timeout=30,
    )

    existing_texts = set()
    tag_re = re.compile(r"\[\[[^\]]*\]\]|<[^>]*>")
    for line in result.stdout.strip().split("\n"):
        if line:
            cleaned = tag_re.sub("", line).strip().lower()
            if cleaned:
                existing_texts.add(cleaned)

    print(f"  DB has {len(existing_texts)} unique source texts")

    missing = []
    found_count = 0
    for item in items:
        check_text = strip_rich_text_tags(item["en"]).lower()
        if check_text in existing_texts:
            found_count += 1
        else:
            missing.append(item)

    print(f"  Already in DB: {found_count}")
    print(f"  Missing from DB: {len(missing)}")
    return missing


def main():
    parser = argparse.ArgumentParser(description="Extract scene texts for translation")
    parser.add_argument("--check-db", action="store_true",
                        help="Cross-reference against PostgreSQL to find truly missing items")
    parser.add_argument("--dsn", default="postgres://postgres@127.0.0.1:5433/localize_agent?sslmode=disable",
                        help="PostgreSQL DSN")
    parser.add_argument("--output", default=None,
                        help="Output file path")
    parser.add_argument("--scenes-dir", default=None,
                        help="Override scenes directory")
    parser.add_argument("--exclude-scenes", default="DebugArea",
                        help="Comma-separated scene names to exclude (default: DebugArea)")
    args = parser.parse_args()

    scenes_dir = args.scenes_dir or os.path.normpath(EXTRACT_DIR)
    exclude_scenes = set(s.strip() for s in args.exclude_scenes.split(",") if s.strip())
    if not os.path.isdir(scenes_dir):
        print(f"ERROR: Scenes directory not found: {scenes_dir}")
        sys.exit(1)

    # Step 1: Extract from all scene files
    print(f"Scanning scenes in: {scenes_dir}")
    if exclude_scenes:
        print(f"  Excluding: {', '.join(sorted(exclude_scenes))}")
    all_texts = []
    scene_files = []
    for root, dirs, files in os.walk(scenes_dir):
        for fname in files:
            if fname.endswith(".unity"):
                scene_files.append(os.path.join(root, fname))

    print(f"  Found {len(scene_files)} scene files")

    for sf in scene_files:
        scene_name = os.path.splitext(os.path.basename(sf))[0]
        if scene_name in exclude_scenes:
            continue
        texts = extract_m_text_from_scene(sf)
        all_texts.extend(texts)

    print(f"  Extracted {len(all_texts)} raw m_text values")

    # Step 2: Deduplicate by text content
    seen = {}
    for item in all_texts:
        key = item["raw_text"]
        if key not in seen:
            seen[key] = item
        else:
            # Track which scenes contain this text
            if "extra_scenes" not in seen[key]:
                seen[key]["extra_scenes"] = []
            if item["scene_file"] not in seen[key]["extra_scenes"]:
                seen[key]["extra_scenes"].append(item["scene_file"])

    unique_texts = list(seen.values())
    print(f"  Unique texts: {len(unique_texts)}")

    # Step 2b: Add manual supplement items (runtime-observed but only in excluded scenes)
    supplement_path = os.path.join(os.path.normpath(OUTPUT_DIR), "scene_supplement.json")
    if os.path.isfile(supplement_path):
        with open(supplement_path, "r", encoding="utf-8") as f:
            supplement = json.load(f)
        for entry in supplement:
            text = entry["en"]
            if text not in seen:
                seen[text] = {
                    "raw_text": text,
                    "scene_file": entry.get("scene_hint", "supplement"),
                }
                unique_texts.append(seen[text])
        print(f"  After supplement: {len(unique_texts)}")

    # Step 3: Filter translatable
    translatable = [t for t in unique_texts if is_translatable(t["raw_text"])]
    print(f"  Translatable: {len(translatable)}")

    # Step 4: Build pipeline items
    items = []
    for t in translatable:
        text = t["raw_text"]
        scene = t["scene_file"]
        item_id = generate_id(text, scene)
        role = classify_text(strip_rich_text_tags(text), scene)

        items.append({
            "id": item_id,
            "en": text,
            "source_raw": text,
            "source_file": f"{scene}.unity",
            "source_type": "scene_m_text",
            "scene_hint": scene,
            "text_role": role,
            "translation_lane": "high",
            "risk": "low",
            "speaker_hint": "",
            "context_en": "",
            "prev_en": "",
            "next_en": "",
            "meta_path_label": f"{scene}/m_text",
        })

    print(f"  Pipeline items: {len(items)}")

    # Step 5: Optionally check DB
    if args.check_db:
        print("\nChecking against PostgreSQL...")
        items = check_db_existence(items, args.dsn)

    # Step 6: Output
    output_path = args.output or os.path.join(
        os.path.normpath(OUTPUT_DIR), "scene_overlay_package.json"
    )

    package = {
        "format": "esoteric-ebb-scene-overlay.v1",
        "extracted_at": __import__("datetime").datetime.now().isoformat(),
        "source": "Unity Scene m_text extraction",
        "total_items": len(items),
        "items": items,
    }

    os.makedirs(os.path.dirname(output_path), exist_ok=True)
    with open(output_path, "w", encoding="utf-8") as f:
        json.dump(package, f, ensure_ascii=False, indent=2)

    print(f"\nOutput: {output_path}")
    print(f"Total items for translation: {len(items)}")

    # Print category breakdown
    roles = {}
    for item in items:
        r = item["text_role"]
        roles[r] = roles.get(r, 0) + 1
    print("\nCategory breakdown:")
    for r, c in sorted(roles.items(), key=lambda x: -x[1]):
        print(f"  {r}: {c}")


if __name__ == "__main__":
    main()
