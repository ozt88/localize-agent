"""Provider-neutral accounting for cumulative session usage snapshots.

Input includes cached input; reasoning is a subset of output. Neither is added
again. No pricing or account-quota equivalence is inferred.
"""
FIELDS=('input','cached_input','output','reasoning_output')


def aggregate(snapshots,expected_sessions=(),*,uncertain_sessions=()):
    sessions={};regressions=[]
    for snapshot in snapshots:
        key=snapshot['session_id']
        if not isinstance(key,str) or not key:raise ValueError('Usage session identity required')
        values={f:snapshot[f] for f in FIELDS}
        if any(type(v) is not int or v<0 for v in values.values()):raise ValueError('Invalid usage count')
        if values['cached_input']>values['input'] or values['reasoning_output']>values['output']:
            raise ValueError('Usage subset exceeds total')
        old=sessions.get(key)
        if old and any(values[f]<old[f] for f in FIELDS):
            regressions.append(key)
        sessions[key]={f:max(values[f],old[f] if old else 0) for f in FIELDS}
    total={f:sum(v[f] for v in sessions.values()) for f in FIELDS}
    total['total']=total['input']+total['output']
    total['uncached_input']=total['input']-total['cached_input']
    missing=sorted(set(expected_sessions)-set(sessions))
    uncertain=sorted(set(uncertain_sessions))
    return {'tokens':total,'sessions':sessions,'missing_sessions':missing,
            'uncertain_sessions':uncertain,
            'regressed_sessions':sorted(set(regressions)),
            'complete':not missing and not regressions and not uncertain and bool(sessions),
            'quota_equivalence':'unknown'}
