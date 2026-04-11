#!/usr/bin/env python3
"""
RAG Context Injection A/B Test (v3 - full rerun both conditions)
================================================================
A 조건(No RAG)과 B 조건(With RAG) 모두 현재 스코어러로 재번역+재측정하여
공정하게 비교.

기존 ab_test_rag_results.json에서 배치 ID를 읽어와 동일 10개 배치 재사용.
(파일 없으면 DB에서 seed=42로 새로 선택)

Phase 07.1 Plan 04: D-20 검증 — RAG 주입이 번역 품질을 저하시키지 않음을 확인.

Usage:
    python ab_test_rag.py
"""

import json
import os
import random
import subprocess
import sys
import time
from pathlib import Path

sys.stdout.reconfigure(encoding="utf-8", errors="replace")
sys.stderr.reconfigure(encoding="utf-8", errors="replace")

SCRIPT_DIR = Path(__file__).parent  # projects/esoteric-ebb/context/
PROJECT_DIR = SCRIPT_DIR.parent     # projects/esoteric-ebb/
PROJECT_ROOT = PROJECT_DIR.parent.parent  # localize-agent/
PSQL = r"C:\Program Files\PostgreSQL\17\bin\psql.exe"
DSN = "host=localhost port=5433 user=postgres dbname=localize_agent sslmode=disable"
RAG_CONTEXT_PATH = str(PROJECT_DIR / "rag" / "rag_batch_context.json")
RESULTS_PATH = SCRIPT_DIR / "ab_test_rag_results.json"

NUM_BATCHES = 10


def psql(query: str) -> str:
    """Run a psql query and return stdout."""
    result = subprocess.run(
        [PSQL, "-h", "localhost", "-p", "5433", "-U", "postgres",
         "-d", "localize_agent", "-t", "-A", "-c", query],
        capture_output=True, text=True, encoding="utf-8"
    )
    if result.returncode != 0:
        print(f"  psql error: {result.stderr.strip()}", file=sys.stderr)
    return result.stdout.strip()


def load_batch_ids_from_results() -> list[str] | None:
    """ab_test_rag_results.json에서 배치 ID 목록 로드 (있으면)."""
    if RESULTS_PATH.exists():
        data = json.load(open(RESULTS_PATH, encoding="utf-8"))
        ids = [b["batch_id"] for b in data.get("batches", [])]
        if ids:
            print(f"  Reusing {len(ids)} batch IDs from existing results file.")
            return ids
    return None


def select_test_batches() -> list[str]:
    """done 상태이면서 RAG hints가 있는 배치 중 10개 선택 (seed=42)."""
    rag = json.load(open(RAG_CONTEXT_PATH, encoding="utf-8"))
    hints_set = {k for k, v in rag.items() if v and k.strip()}

    result = psql("""
        SELECT batch_id, AVG(score_final) as avg_score, count(*) as cnt
        FROM pipeline_items_v2
        WHERE state = 'done' AND score_final > 0
        GROUP BY batch_id
        HAVING count(*) >= 5
        ORDER BY batch_id
    """)

    candidates = []
    for line in result.strip().split("\n"):
        if "|" not in line:
            continue
        parts = line.split("|")
        bid = parts[0].strip()
        if bid in hints_set:
            candidates.append(bid)

    random.seed(42)
    selected = random.sample(candidates, min(NUM_BATCHES, len(candidates)))
    return selected


def reset_batches(batch_ids: list[str]) -> int:
    """배치를 pending_translate 상태로 리셋."""
    quoted = ",".join(f"'{b}'" for b in batch_ids)
    query = f"""
        UPDATE pipeline_items_v2
        SET state = 'pending_translate',
            ko_raw = NULL,
            ko_formatted = NULL,
            score_final = -1,
            translate_attempts = 0,
            format_attempts = 0,
            score_attempts = 0,
            failure_type = '',
            last_error = '',
            claimed_by = '',
            claimed_at = NOW(),
            lease_until = NOW()
        WHERE batch_id IN ({quoted})
        RETURNING id
    """
    result = psql(query)
    return len(result.strip().split("\n")) if result.strip() else 0


def get_scores(batch_ids: list[str]) -> dict[str, float]:
    """배치별 평균 score_final."""
    quoted = ",".join(f"'{b}'" for b in batch_ids)
    result = psql(f"""
        SELECT batch_id, AVG(score_final)
        FROM pipeline_items_v2
        WHERE batch_id IN ({quoted}) AND score_final > 0
        GROUP BY batch_id
    """)
    scores = {}
    for line in result.strip().split("\n"):
        if "|" in line:
            bid, score = line.split("|", 1)
            scores[bid.strip()] = float(score.strip())
    return scores


def run_pipeline_stage(stage: str, rag_path: str):
    """파이프라인 한 스테이지를 실행 (모든 pending+working 아이템 처리까지 반복)."""
    pipeline_exe = str(PROJECT_ROOT / "workflow" / ".bin" / "go-v2-pipeline.exe")

    lease_sec = 150
    pass_timeout = 180

    cmd = [
        pipeline_exe,
        "--backend", "postgres",
        "--dsn", DSN,
        "--role", stage,
        "--once",
        "--idle-sleep-sec", "1",
        "--lease-sec", str(lease_sec),
        f"--{stage}-concurrency", "2",
        f"--{stage}-timeout-sec", "120",
    ]
    if rag_path and stage in ("translate", "score"):
        cmd.extend(["--rag-context", rag_path])

    max_passes = 30
    for p in range(max_passes):
        try:
            result = subprocess.run(cmd, capture_output=True, text=True, encoding="utf-8", timeout=pass_timeout)
            print(f"    {stage} pass {p+1}: exit={result.returncode}")
            if result.stdout:
                for line in result.stdout.split("\n"):
                    if "claim" in line.lower() or "pending" in line.lower() or "done" in line.lower() or "state" in line.lower():
                        print(f"      {line.strip()}")
        except subprocess.TimeoutExpired:
            print(f"    {stage} pass {p+1}: timeout (expected when queue empties with --once)")
        time.sleep(1)

        pending_name = f"pending_{stage}"
        working_name = f"working_{stage}"
        rem_pending_raw = psql(f"SELECT count(*) FROM pipeline_items_v2 WHERE state = '{pending_name}'")
        rem_working_raw = psql(f"SELECT count(*) FROM pipeline_items_v2 WHERE state = '{working_name}'")
        rem_pending = int(rem_pending_raw.strip()) if rem_pending_raw.strip().isdigit() else 0
        rem_working = int(rem_working_raw.strip()) if rem_working_raw.strip().isdigit() else 0

        print(f"    {stage} remaining: pending={rem_pending} working={rem_working}")

        if rem_pending == 0 and rem_working == 0:
            print(f"    {stage}: all items processed")
            break

        if rem_pending == 0 and rem_working > 0:
            print(f"    {stage}: {rem_working} working items stuck — running cleanup-stale-claims")
            cleanup_cmd = [
                pipeline_exe,
                "--backend", "postgres",
                "--dsn", DSN,
                "--role", stage,
                "--once",
                "--lease-sec", "1",
                "--cleanup-stale-claims",
                f"--{stage}-concurrency", "1",
                f"--{stage}-timeout-sec", "5",
            ]
            try:
                subprocess.run(cleanup_cmd, capture_output=True, text=True, encoding="utf-8", timeout=30)
            except subprocess.TimeoutExpired:
                pass
            time.sleep(2)


def build_pipeline():
    """파이프라인 빌드."""
    print("  Building pipeline...")
    build = subprocess.run(
        ["go", "build", "-o", ".bin/go-v2-pipeline.exe", "./cmd/go-v2-pipeline"],
        cwd=str(PROJECT_ROOT / "workflow"),
        capture_output=True, text=True
    )
    if build.returncode != 0:
        print(f"  Build error: {build.stderr}", file=sys.stderr)
        return False
    return True


def run_condition(label: str, batch_ids: list[str], rag_path: str) -> dict[str, float]:
    """한 조건(A 또는 B)에 대해 reset → translate → format → score 실행 후 스코어 반환."""
    print(f"\n--- Condition {label}: {'No RAG' if not rag_path else 'With RAG'} ---")
    count = reset_batches(batch_ids)
    print(f"  Reset {count} items to pending_translate")

    print("\n  Stage: translate")
    run_pipeline_stage("translate", rag_path)

    print("\n  Stage: format")
    run_pipeline_stage("format", "")

    print("\n  Stage: score")
    run_pipeline_stage("score", rag_path)

    scores = get_scores(batch_ids)
    print(f"  Scores collected: {len(scores)} batches")
    return scores


def main():
    print("=== RAG Context Injection A/B Test (v3 - full rerun) ===\n")

    # 1. Determine test batches
    batch_ids = load_batch_ids_from_results()
    if batch_ids is None:
        batch_ids = select_test_batches()
        if len(batch_ids) < NUM_BATCHES:
            print(f"WARNING: only {len(batch_ids)} eligible batches (need {NUM_BATCHES})")

    print(f"\nTest batches ({len(batch_ids)}):")
    for bid in batch_ids:
        print(f"  {bid}")

    # 2. Build once
    if not build_pipeline():
        return

    # 3. Condition A: No RAG (rerun with current scorer)
    no_rag_scores = run_condition("A", batch_ids, "")

    # 4. Condition B: With RAG
    rag_scores = run_condition("B", batch_ids, RAG_CONTEXT_PATH)

    # 5. Compare
    print("\n\n=== Results ===\n")
    print(f"{'Batch ID':<80} | {'No RAG':>7} | {'With RAG':>8} | {'Delta':>6}")
    print("-" * 110)

    results = []
    for bid in batch_ids:
        sa = no_rag_scores.get(bid, 0)
        sb = rag_scores.get(bid, 0)
        delta = sb - sa
        results.append({
            "batch_id": bid,
            "score_no_rag": round(sa, 2),
            "score_with_rag": round(sb, 2),
            "delta": round(delta, 2)
        })
        print(f"{bid:<80} | {sa:>7.1f} | {sb:>8.1f} | {delta:>+6.1f}")

    scored = [r for r in results if r["score_no_rag"] > 0 and r["score_with_rag"] > 0]
    avg_a = sum(r["score_no_rag"] for r in scored) / len(scored) if scored else 0
    avg_b = sum(r["score_with_rag"] for r in scored) / len(scored) if scored else 0
    avg_delta = avg_b - avg_a

    print("-" * 110)
    print(f"{'Average (scored only)':<80} | {avg_a:>7.1f} | {avg_b:>8.1f} | {avg_delta:>+6.1f}")
    print(f"Scored batches: {len(scored)}/{len(results)}")
    print(f"\nVerdict: {'PASS' if avg_delta >= -0.5 else 'FAIL'} (avg delta = {avg_delta:+.2f}, threshold >= -0.5)")

    # Save results
    output = {
        "batches": results,
        "avg_no_rag": round(avg_a, 2),
        "avg_with_rag": round(avg_b, 2),
        "avg_delta": round(avg_delta, 2),
        "verdict": "PASS" if avg_delta >= -0.5 else "FAIL",
        "scored_batches": len(scored),
        "total_batches": len(results),
        "timestamp": time.strftime("%Y-%m-%dT%H:%M:%S"),
        "note": "v3: both conditions fully rerun with current scorer"
    }
    with open(RESULTS_PATH, "w", encoding="utf-8") as f:
        json.dump(output, f, ensure_ascii=False, indent=2)
    print(f"\nResults saved to: {RESULTS_PATH}")


if __name__ == "__main__":
    main()
