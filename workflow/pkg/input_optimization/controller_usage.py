"""Scope controller rollout usage to one explicit user request, excluding inherited totals."""
import json


def measure(path, boundary_text):
    latest=baseline=None;matches=[];timestamp=None
    with path.open(encoding='utf-8') as stream:
        for line in stream:
            event=json.loads(line);payload=event.get('payload',{})
            if event.get('type')=='event_msg' and payload.get('type')=='token_count' and payload.get('info'):
                latest=payload['info'].get('total_token_usage');timestamp=event.get('timestamp')
            if event.get('type')=='response_item' and payload.get('type')=='message' and payload.get('role')=='user':
                text=' '.join(c.get('text','') for c in payload.get('content',[]))
                if boundary_text in text:
                    baseline=latest;matches.append(event.get('timestamp'))
    if len(matches)!=1 or not baseline or not latest:
        raise ValueError('Unique request usage boundary unavailable')
    delta={k:latest[k]-baseline[k] for k in latest if isinstance(latest[k],int)}
    if any(v<0 for v in delta.values()):raise ValueError('Usage counter reset')
    delta['uncached_input_tokens']=delta['input_tokens']-delta['cached_input_tokens']
    if delta['total_tokens']!=delta['input_tokens']+delta['output_tokens']:
        raise ValueError('Usage total inconsistent')
    return dict(source=str(path),request_timestamp=matches[0],measured_through=timestamp,
                tokens=delta,scope='controller session only; worker sessions excluded',
                final_response_not_included=True,quota_equivalence='unknown')
