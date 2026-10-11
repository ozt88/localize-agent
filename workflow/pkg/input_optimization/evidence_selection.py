"""Auditable, opt-in evidence views. Selection is NOT semantic equivalence.

Game adapters explicitly classify optional dictionary fields. The source and all
omitted values stay in a hash-bound archive, allowing exact restoration or
expansion. Unknown dependencies must be reported, never silently discarded.
"""
import copy
from .core import digest, canonical


def _parent(value,path):
    if not path or not isinstance(path[-1],str):raise ValueError('Only dictionary fields may be deferred')
    for part in path[:-1]:
        if isinstance(value,list):
            if type(part) is not int or not 0<=part<len(value):raise ValueError('Invalid array path')
        elif not isinstance(value,dict) or not isinstance(part,str):raise ValueError('Invalid object path')
        value=value[part]
    if not isinstance(value,dict):raise ValueError('Field parent is not an object')
    return value


def select(source,deferrals,required_paths=(),unresolved=()):
    """Move explicit fields to an external archive; fail closed on required paths."""
    view=copy.deepcopy(source);entries=[];paths=[]
    for rule in deferrals:
        path=tuple(rule['path']);reason=rule.get('reason')
        if not isinstance(reason,str) or not reason.strip():raise ValueError('Deferral reason required')
        for other in paths:
            if path[:len(other)]==other or other[:len(path)]==path:raise ValueError('Overlapping deferrals')
        for required in required_paths:
            required=tuple(required)
            if required[:len(path)]==path or path[:len(required)]==required:
                raise ValueError('Required evidence cannot be deferred')
        parent=_parent(view,path)
        if path[-1] not in parent:raise ValueError('Deferred field absent')
        value=parent.pop(path[-1]);paths.append(path)
        entry={'path':list(path),'reason':reason,'value':value,'sha256':digest(value)}
        if 'keep_indices' in rule:
            indices=rule['keep_indices']
            if not isinstance(value,list) or not isinstance(indices,list) or any(type(i) is not int or not 0<=i<len(value) for i in indices) or indices!=sorted(set(indices)):
                raise ValueError('Selection indices must preserve original order without duplicates')
            parent[path[-1]]=[copy.deepcopy(value[i]) for i in indices]
            entry['keep_indices']=indices;entry['replacement_sha256']=digest(parent[path[-1]])
        entries.append(entry)
    archive={'version':'evidence-selection-v1','source_sha256':digest(source),'view_sha256':digest(view),
             'entries':entries,'required_paths':[list(p) for p in required_paths],
             'unresolved':copy.deepcopy(list(unresolved)),'semantic_equivalence_verified':False}
    if canonical(restore(view,archive))!=canonical(source):raise ValueError('Evidence restoration failed')
    return view,archive


def restore(view,archive):
    if archive.get('version')!='evidence-selection-v1' or digest(view)!=archive['view_sha256']:
        raise ValueError('Evidence view changed')
    result=copy.deepcopy(view)
    for entry in archive['entries']:
        path=entry['path'];parent=_parent(result,path)
        if digest(entry['value'])!=entry['sha256']:raise ValueError('Archive entry changed')
        if 'keep_indices' in entry:
            if path[-1] not in parent or digest(parent[path[-1]])!=entry['replacement_sha256']:
                raise ValueError('Selected list changed')
            if canonical(parent[path[-1]])!=canonical([entry['value'][i] for i in entry['keep_indices']]):
                raise ValueError('Selected list binding changed')
        elif path[-1] in parent:raise ValueError('Deferred field restored without a new binding')
        parent[path[-1]]=copy.deepcopy(entry['value'])
    if digest(result)!=archive['source_sha256']:raise ValueError('Source archive changed')
    return result


def expand(view,archive,paths):
    """Return a newly hash-bound view with requested evidence restored inline."""
    source=restore(view,archive);wanted={tuple(p) for p in paths}
    existing={tuple(e['path']) for e in archive['entries']}
    if not wanted<=existing:raise ValueError('Unknown expansion path')
    remaining=[{k:e[k] for k in ('path','reason','keep_indices') if k in e} for e in archive['entries'] if tuple(e['path']) not in wanted]
    return select(source,remaining,archive['required_paths'],archive['unresolved'])
