import argparse
import json
import pathlib
import sys
import time
import urllib.parse
import urllib.request


def load_json(path):
    return json.loads(pathlib.Path(path).read_text(encoding="utf-8"))


def unique_candidates(dictionary, include_categories=None):
    buckets = [
        "spell_glossary_terms",
        "item_prefab_candidates",
        "spell_name_candidates",
        "runtime_ui_name_candidates",
        "system_translate_terms",
        "proper_noun_terms",
        "faction_terms",
        "place_terms",
        "lore_terms",
    ]
    out = []
    seen = set()
    for bucket in buckets:
        if include_categories and bucket not in include_categories:
            continue
        for item in dictionary.get(bucket, []):
            if isinstance(item, str):
                source = item.strip()
                desc = ""
            else:
                source = (item.get("source") or item.get("name") or "").strip()
                desc = item.get("description") or item.get("note") or ""
            if not source:
                continue
            key = source.lower()
            if key in seen:
                continue
            seen.add(key)
            out.append({
                "source": source,
                "category": bucket,
                "description": desc,
                "translation_policy": item.get("translation_policy", "translate") if isinstance(item, dict) else "translate",
                "source_file": item.get("source_file", "") if isinstance(item, dict) else "",
            })
    return out


def create_session(server_url, directory):
    body = json.dumps({}).encode("utf-8")
    req = urllib.request.Request(
        server_url.rstrip("/") + "/session?" + urllib.parse.urlencode({"directory": directory}),
        data=body,
        headers={"content-type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=120) as resp:
        payload = json.loads(resp.read().decode("utf-8"))
    sid = payload.get("id", "")
    if not sid:
        raise RuntimeError("session id missing")
    return sid


def warmup_session(server_url, directory, session_id, model, agent):
    provider_id, model_id = model.split("/", 1)
    warmup = (
        "Reply with exactly OK. "
        "You are generating glossary suggestions for a Korean game localization project. "
        "When asked later, you must return only one JSON array. "
        "Each element must be an object with keys source, target, mode. "
        "mode is either translate or preserve. "
        "If mode is preserve, target must equal source exactly."
    )
    body = {
        "model": {"providerID": provider_id, "modelID": model_id},
        "parts": [{"type": "text", "text": warmup}],
    }
    if agent:
        body["agent"] = agent
    raw = json.dumps(body).encode("utf-8")
    url = server_url.rstrip("/") + f"/session/{session_id}/message?" + urllib.parse.urlencode({"directory": directory})
    req = urllib.request.Request(
        url,
        data=raw,
        headers={"content-type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=120) as resp:
        payload = json.loads(resp.read().decode("utf-8"))
    parts = payload.get("parts", [])
    texts = []
    for part in parts:
        if isinstance(part, dict) and part.get("type") == "text":
            texts.append(part.get("text", ""))
    return "\n".join(texts).strip()


def send_message(server_url, directory, session_id, model, agent, prompt, timeout_sec):
    provider_id, model_id = model.split("/", 1)
    body = {
        "model": {"providerID": provider_id, "modelID": model_id},
        "parts": [{"type": "text", "text": prompt}],
    }
    if agent:
        body["agent"] = agent
    raw = json.dumps(body).encode("utf-8")
    url = server_url.rstrip("/") + f"/session/{session_id}/message?" + urllib.parse.urlencode({"directory": directory})
    req = urllib.request.Request(
        url,
        data=raw,
        headers={"content-type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=timeout_sec) as resp:
        payload = json.loads(resp.read().decode("utf-8"))
    parts = payload.get("parts", [])
    texts = []
    for part in parts:
        if isinstance(part, dict) and part.get("type") == "text":
            texts.append(part.get("text", ""))
    if not texts:
        raise RuntimeError("no text in response")
    return "\n".join(texts).strip()


def build_prompt(batch):
    rules = (
        "Return exactly one JSON array and nothing else. "
        "Each element must be an object with keys: source, target, mode. "
        "mode must be either 'translate' or 'preserve'. "
        "If mode is 'preserve', target must equal source exactly. "
        "Use Korean fixed terminology suitable for a game localization glossary. "
        "Prefer short, stable Korean renderings. "
        "For spells, items, skills, UI labels, factions, places, and lore terms, propose one fixed Korean term. "
        "Do not preserve raw English unless it is clearly better kept as-is, such as an acronym or identifier-like name. "
        "Proper nouns are usually translate/transliterate, not preserve, unless they are clearly best kept in raw English. "
        "Control tokens are not included in this task. "
        "Category and short note are hints only; do not repeat them in target."
    )
    payload = {
        "items": batch,
    }
    return rules + "\nInput JSON: " + json.dumps(payload, ensure_ascii=False)


def parse_response(raw):
    raw = raw.strip()
    if raw.startswith("```json"):
        raw = raw[len("```json"):].strip()
    if raw.startswith("```"):
        raw = raw[len("```"):].strip()
    if raw.endswith("```"):
        raw = raw[:-3].strip()
    data = json.loads(raw)
    if not isinstance(data, list):
        raise RuntimeError("response is not a JSON array")
    out = []
    for item in data:
        if not isinstance(item, dict):
            continue
        source = str(item.get("source", "")).strip()
        target = str(item.get("target", "")).strip()
        mode = str(item.get("mode", "")).strip()
        if source and target and mode in {"translate", "preserve"}:
            out.append({
                "source": source,
                "target": target if mode == "translate" else source,
                "mode": mode,
            })
    return out


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--dictionary", required=True)
    parser.add_argument("--preserve", required=True)
    parser.add_argument("--base-curated", required=True)
    parser.add_argument("--out", required=True)
    parser.add_argument("--server-url", default="http://127.0.0.1:4112")
    parser.add_argument("--model", default="openai/gpt-5.4")
    parser.add_argument("--agent", default="rt-ko-translate-primary")
    parser.add_argument("--batch-size", type=int, default=40)
    parser.add_argument("--start-index", type=int, default=0)
    parser.add_argument("--max-items", type=int, default=0)
    parser.add_argument("--timeout-sec", type=int, default=300)
    parser.add_argument("--include-categories", default="")
    parser.add_argument("--progress-log", default="")
    args = parser.parse_args()

    dictionary = load_json(args.dictionary)
    preserve = load_json(args.preserve)
    base = load_json(args.base_curated)

    base_translate = {item["source"].lower(): item for item in base.get("translate_terms", [])}
    base_preserve = list(base.get("preserve_terms", []))

    include_categories = {x.strip() for x in args.include_categories.split(",") if x.strip()}
    candidates = unique_candidates(dictionary, include_categories if include_categories else None)
    unresolved = []
    for item in candidates:
        key = item["source"].lower()
        if key in base_translate:
            continue
        unresolved.append(item)
    start_index = max(0, args.start_index)
    if start_index:
        unresolved = unresolved[start_index:]
    if args.max_items and args.max_items > 0:
        unresolved = unresolved[:args.max_items]

    out_path = pathlib.Path(args.out)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    progress_path = pathlib.Path(args.progress_log) if args.progress_log else None
    if progress_path is not None:
        progress_path.parent.mkdir(parents=True, exist_ok=True)

    directory = str(pathlib.Path.cwd())
    suggestions = []
    total = len(unresolved)
    result = {
        "format": "esoteric-ebb-glossary-autosuggest.v1",
        "created_at": time.strftime("%Y-%m-%dT%H:%M:%S"),
        "source_dictionary": args.dictionary,
        "source_preserve_terms": args.preserve,
        "base_curated": args.base_curated,
        "notes": [
            "Expanded glossary artifact with autosuggested targets for unresolved terms.",
            "Base curated exact-live targets are preserved as-is.",
            "Autosuggested entries should be reviewed before hard enforcement.",
        ],
        "translate_terms": list(base.get("translate_terms", [])),
        "preserve_terms": base_preserve + preserve.get("preserve_exact", []),
        "counts": {
            "base_translate_terms": len(base.get("translate_terms", [])),
            "autosuggested_total": 0,
            "autosuggested_translate": 0,
            "autosuggested_preserve": 0,
            "start_index": start_index,
            "max_items": args.max_items,
        },
    }
    out_path.write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8")
    session_id = create_session(args.server_url, directory)
    warmup_reply = warmup_session(args.server_url, directory, session_id, args.model, args.agent)
    if progress_path is not None:
        progress_path.write_text(f"warmup={warmup_reply}\n", encoding="utf-8")
    for start in range(0, total, args.batch_size):
        batch = unresolved[start:start + args.batch_size]
        compact_batch = []
        for item in batch:
            compact_batch.append({
                "source": item["source"],
                "category": item["category"],
                "description": (item.get("description") or "")[:180],
            })
        prompt = build_prompt(compact_batch)
        raw = send_message(args.server_url, directory, session_id, args.model, args.agent, prompt, args.timeout_sec)
        parsed = parse_response(raw)
        parsed_map = {item["source"].lower(): item for item in parsed}
        for item in batch:
            hit = parsed_map.get(item["source"].lower())
            if not hit:
                continue
            row = dict(item)
            row.update({
                "target": hit["target"],
                "mode": hit["mode"],
                "provenance": "autosuggest_llm",
            })
            suggestions.append(row)
        result["translate_terms"] = list(base.get("translate_terms", [])) + [s for s in suggestions if s.get("mode") == "translate"]
        result["preserve_terms"] = base_preserve + preserve.get("preserve_exact", []) + [s for s in suggestions if s.get("mode") == "preserve"]
        result["counts"] = {
            "base_translate_terms": len(base.get("translate_terms", [])),
            "autosuggested_total": len(suggestions),
            "autosuggested_translate": len([s for s in suggestions if s.get("mode") == "translate"]),
            "autosuggested_preserve": len([s for s in suggestions if s.get("mode") == "preserve"]),
            "start_index": start_index,
            "max_items": args.max_items,
        }
        out_path.write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8")
        if progress_path is not None:
            progress_path.write_text(
                f"warmup={warmup_reply}\n{start + len(batch)}/{total} batches_done={(start // args.batch_size) + 1} autosuggested={len(suggestions)}\n",
                encoding="utf-8",
            )

    print(str(out_path))


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(f"error: {exc}", file=sys.stderr)
        sys.exit(1)
