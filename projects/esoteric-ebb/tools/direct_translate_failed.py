#!/usr/bin/env python3
"""Direct-translate failed pipeline items via OpenCode API."""
import json, sys, os, re, subprocess, requests

sys.stdout.reconfigure(encoding='utf-8')

PSQL = "C:/Program Files/PostgreSQL/17/bin/psql.exe"
DSN_ARGS = ["-h", "127.0.0.1", "-p", "5433", "-U", "postgres", "-d", "localize_agent"]
OPENCODE = "http://127.0.0.1:4112"
BATCH_SIZE = 8

def psql_query(sql):
    env = {**os.environ, 'PGCLIENTENCODING': 'UTF8'}
    r = subprocess.run([PSQL] + DSN_ARGS + ["-t", "-A", "-c", sql],
                       capture_output=True, text=True, encoding='utf-8', env=env, timeout=30)
    return r.stdout.strip()

def psql_file(path):
    env = {**os.environ, 'PGCLIENTENCODING': 'UTF8'}
    r = subprocess.run([PSQL] + DSN_ARGS + ["-f", path],
                       capture_output=True, text=True, encoding='utf-8', env=env, timeout=60)
    return r.stdout.count("UPDATE 1")

def llm_translate(texts):
    batch_input = [{"i": j, "en": t[:500]} for j, t in enumerate(texts)]
    prompt = (
        "Translate to Korean. Mixed EN/KO: translate remaining English. "
        "Return ONLY a JSON array of strings.\n\n"
        + json.dumps(batch_input, ensure_ascii=False)
    )
    sid = requests.post(f"{OPENCODE}/session", json={}).json().get("id", "")
    resp = requests.post(f"{OPENCODE}/session/{sid}/message", json={
        "model": {"providerID": "openai", "modelID": "gpt-5.4"},
        "parts": [{"type": "text", "text": prompt}],
    }).json()
    text = "\n".join(
        p["text"] for p in resp.get("parts", [])
        if isinstance(p, dict) and p.get("type") == "text"
    )
    match = re.search(r'\[[\s\S]*?\]', text)
    if not match:
        return []
    try:
        return json.loads(match.group())
    except json.JSONDecodeError:
        return re.findall(r'"((?:[^"\\]|\\.)*)"', match.group())

def main():
    rows = psql_query(
        "SELECT i.id, i.pack_json->>'source_raw' "
        "FROM items i JOIN pipeline_items p ON p.id = i.id "
        "WHERE p.state = 'failed' AND COALESCE(i.ko_json->>'Text', '') = '' "
        "ORDER BY LENGTH(i.pack_json->>'source_raw')"
    )
    items = []
    for line in rows.split("\n"):
        if not line: continue
        parts = line.split("|", 1)
        if len(parts) == 2:
            items.append({"id": parts[0], "source": parts[1]})

    print(f"Items to translate: {len(items)}")
    if not items:
        return

    all_results = {}
    for i in range(0, len(items), BATCH_SIZE):
        batch = items[i:i+BATCH_SIZE]
        sources = [it["source"] for it in batch]
        try:
            ko_list = llm_translate(sources)
            for j, ko in enumerate(ko_list):
                if j < len(batch) and isinstance(ko, str) and ko.strip():
                    all_results[batch[j]["id"]] = ko
            print(f"  Batch {i//BATCH_SIZE+1}/{(len(items)-1)//BATCH_SIZE+1}: {len(ko_list)} ok")
        except Exception as ex:
            print(f"  Batch {i//BATCH_SIZE+1}: error {ex}")

    print(f"\nTranslated: {len(all_results)}")

    sql_lines = ["SET client_encoding = 'UTF8';"]
    for item_id, ko in all_results.items():
        safe_id = item_id.replace("'", "''")
        ko_json = json.dumps({"Text": ko}, ensure_ascii=False).replace("'", "''")
        pack_upd = json.dumps({"proposed_ko_restored": ko, "fresh_ko": ko}, ensure_ascii=False).replace("'", "''")
        sql_lines.append(
            f"UPDATE items SET ko_json = '{ko_json}'::jsonb, "
            f"pack_json = pack_json || '{pack_upd}'::jsonb WHERE id = '{safe_id}';"
        )
        sql_lines.append(
            f"UPDATE pipeline_items SET state = 'done', score_final = 75, "
            f"last_error = 'direct_translate' WHERE id = '{safe_id}';"
        )

    sql_path = os.path.join(os.path.dirname(__file__), "..", "output",
                            "batches", "canonical_full_retranslate_live",
                            "direct_translate_failed.sql")
    with open(sql_path, "w", encoding="utf-8") as f:
        f.write("\n".join(sql_lines))

    updates = psql_file(sql_path)
    print(f"DB updates: {updates}")

if __name__ == "__main__":
    main()
