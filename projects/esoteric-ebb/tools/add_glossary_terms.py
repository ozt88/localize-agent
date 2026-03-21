#!/usr/bin/env python3
"""Add missing game terms to the glossary."""
import json
import sys

GLOSSARY_PATH = "projects/esoteric-ebb/patch/output/source_overlay_analysis/esoteric_ebb_glossary_user_merged_20260316.json"

new_translate = [
    # Game world concepts
    {"source": "Fray", "target": "갈래", "mode": "translate", "category": "game_world_term",
     "description": "Core world concept - cosmic threads/paths that unravel. A Fray = a divergent path/thread of fate."},
    {"source": "frayed", "target": "갈라진", "mode": "translate", "category": "game_world_term",
     "description": "Adjective form of Fray - unraveled, split apart like cosmic threads."},
    {"source": "Frays", "target": "갈래들", "mode": "translate", "category": "game_world_term",
     "description": "Plural of Fray."},
    {"source": "Esoteric", "target": "비전의", "mode": "translate", "category": "game_world_term",
     "description": "Core adjective - hidden/arcane magical knowledge. Esoteric Coast, esoteric pockets, esoteric cloth."},
    {"source": "Esoterically", "target": "비전적으로", "mode": "translate", "category": "game_world_term",
     "description": "Adverb form of Esoteric."},
    {"source": "Esoteric Coast", "target": "비전 해안", "mode": "translate", "category": "game_world_term",
     "description": "The coastal region where the game takes place."},
    {"source": "Weal", "target": "길조", "mode": "translate", "category": "game_world_term",
     "description": "Fortune/good omen. D&D Augury result (Weal = good outcome)."},
    {"source": "Divine Intervention", "target": "신의 개입", "mode": "translate", "category": "game_mechanic",
     "description": "Cleric class feature - divine power directly intervening."},
    # Political factions
    {"source": "Azgalism", "target": "아즈갈주의", "mode": "translate", "category": "faction_ideology",
     "description": "Political ideology named after Azgal. Expansionist/nationalist."},
    {"source": "Azgalist", "target": "아즈갈주의자", "mode": "translate", "category": "faction_ideology",
     "description": "Follower of Azgalism."},
    {"source": "Azgalian", "target": "아즈갈의", "mode": "translate", "category": "faction_ideology",
     "description": "Adjective relating to Azgal or Azgalism."},
    {"source": "Freestrider", "target": "자유항해단", "mode": "translate", "category": "faction_ideology",
     "description": "Political faction - free traders, smugglers, libertarian. Free+Strider = free wanderers."},
    {"source": "Freestriderism", "target": "자유항해주의", "mode": "translate", "category": "faction_ideology",
     "description": "Ideology of the Freestriders."},
    {"source": "Freestriders", "target": "자유항해단원들", "mode": "translate", "category": "faction_ideology",
     "description": "Members of the Freestrider faction."},
    # Titles/nicknames
    {"source": "The Duck", "target": "오리", "mode": "translate", "category": "character_title",
     "description": "Borgo's title - feared enforcer alias."},
    {"source": "The Hand", "target": "손", "mode": "translate", "category": "character_title",
     "description": "Ost's title - one of three enforcer roles."},
    {"source": "The Eye", "target": "눈", "mode": "translate", "category": "character_title",
     "description": "Razz's title - one of three enforcer roles."},
    {"source": "The Rage", "target": "분노", "mode": "translate", "category": "character_title",
     "description": "Kraaid's title - one of three enforcer roles."},
    # Interjections
    {"source": "Tut", "target": "쯧", "mode": "translate", "category": "interjection",
     "description": "Tongue-clicking disapproval sound."},
]

new_preserve = [
    {"source": "Gorm", "target": "Gorm", "mode": "preserve", "category": "npc_name",
     "description": "NPC name. Do NOT translate as bear."},
    {"source": "Smarter", "target": "Smarter", "mode": "preserve", "category": "npc_name",
     "description": "NPC name. Do NOT translate as adjective."},
    {"source": "Borgo", "target": "Borgo", "mode": "preserve", "category": "npc_name",
     "description": "NPC name, also known as The Duck."},
    {"source": "Snell", "target": "Snell", "mode": "preserve", "category": "npc_name",
     "description": "Companion NPC name."},
    {"source": "Kraaid", "target": "Kraaid", "mode": "preserve", "category": "npc_name",
     "description": "NPC name, The Rage."},
    {"source": "Ettir", "target": "Ettir", "mode": "preserve", "category": "npc_name",
     "description": "Companion NPC name."},
    {"source": "Razz", "target": "Razz", "mode": "preserve", "category": "npc_name",
     "description": "NPC name, The Eye."},
    {"source": "Ost", "target": "Ost", "mode": "preserve", "category": "npc_name",
     "description": "NPC name, The Hand."},
    {"source": "Viira", "target": "Viira", "mode": "preserve", "category": "npc_name",
     "description": "NPC name."},
    {"source": "Alfoz", "target": "Alfoz", "mode": "preserve", "category": "npc_name",
     "description": "NPC name."},
    {"source": "Braxo", "target": "Braxo", "mode": "preserve", "category": "npc_name",
     "description": "NPC name."},
    {"source": "Meek", "target": "Meek", "mode": "preserve", "category": "npc_name",
     "description": "Companion NPC name. Do NOT translate as adjective."},
    {"source": "Azgal", "target": "Azgal", "mode": "preserve", "category": "historical_figure",
     "description": "Historical figure whose ideology spawned Azgalism."},
]

def main():
    with open(GLOSSARY_PATH, "r", encoding="utf-8") as f:
        data = json.load(f)

    existing_sources = set()
    for e in data.get("translate_terms", []):
        existing_sources.add(e["source"].lower())
    for e in data.get("preserve_terms", []):
        existing_sources.add(e["source"].lower())

    added_t = 0
    for term in new_translate:
        if term["source"].lower() not in existing_sources:
            data["translate_terms"].append(term)
            existing_sources.add(term["source"].lower())
            added_t += 1

    added_p = 0
    for term in new_preserve:
        if term["source"].lower() not in existing_sources:
            data["preserve_terms"].append(term)
            existing_sources.add(term["source"].lower())
            added_p += 1

    data["counts"]["translate_terms"] = len(data["translate_terms"])

    with open(GLOSSARY_PATH, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)

    print(f"Added {added_t} translate terms, {added_p} preserve terms")
    print(f"Total: {len(data['translate_terms'])} translate, {len(data['preserve_terms'])} preserve")

if __name__ == "__main__":
    main()
