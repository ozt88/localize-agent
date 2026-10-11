"""Read cumulative provider usage without double-counting repeated snapshots."""
import json
from .cost_accounting import aggregate


def read_usage(directory):
    path = directory / 'trace.jsonl'
    if not path.exists():
        return aggregate([], ['missing_trace'])
    snapshots, sessions = [], []
    interrupted = False
    for line in path.read_text(encoding='utf-8').splitlines():
        event = json.loads(line)
        if event['kind'] not in ('codex_timing', 'codex_session', 'codex_usage'):
            continue
        raw = json.loads(event['response_raw'])
        if event['kind'] == 'codex_timing':
            interrupted |= not raw.get('success', True)
        elif event['kind'] == 'codex_session':
            sessions.append(raw['thread_id'])
        else:
            value = raw['tokenUsage']['total']
            snapshots.append(dict(session_id=raw['threadId'], input=value['inputTokens'],
                                  cached_input=value['cachedInputTokens'], output=value['outputTokens'],
                                  reasoning_output=value['reasoningOutputTokens']))
    return aggregate(snapshots, sessions, uncertain_sessions=sessions if interrupted else ())
