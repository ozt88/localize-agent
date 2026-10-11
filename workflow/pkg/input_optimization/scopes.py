"""Lossless co-reading manifests, independent of any game or model provider.

Adapters supply a complete joint base and original scopes. This module verifies
every inverse, target ownership and immutable provenance; it never approves
translations or writes operational state.
"""
import copy
from difflib import SequenceMatcher
from .core import canonical, digest


DELTA_INSTRUCTIONS = (
    'co_read_context describes original review scopes as exact deltas from this '
    'prepared request with co_read_context removed. Each delta operation has a path '
    'array of object keys or array indices: set replaces/adds value, delete removes '
    'an object key, splice replaces delete_count array members at start with values. '
    'A set value_from_base is a path into the unchanged joint base. Splice entries '
    'contain either base_path into that unchanged base or a literal value. '
    'Apply operations in listed order to a copy of the joint base to recover each '
    'original request. All original evidence remains available. Read related source '
    'scopes jointly using authored edges and conditions, never packaging order. '
    'The joint document target list is authoritative for output; original target '
    'ownership and review scope boundaries remain recorded in this manifest. '
)


def difference(base, value, path=()):
    if canonical(base)==canonical(value):return []
    if isinstance(base,dict) and isinstance(value,dict):
        operations=[]
        for key in sorted(set(base)-set(value)):operations.append({'op':'delete','path':[*path,key]})
        for key in sorted(value):
            if key not in base:operations.append({'op':'set','path':[*path,key],'value':copy.deepcopy(value[key])})
            else:operations.extend(difference(base[key],value[key],(*path,key)))
        return operations
    if isinstance(base,list) and isinstance(value,list):
        operations=[]
        matcher=SequenceMatcher(a=[canonical(x) for x in base],b=[canonical(x) for x in value],autojunk=False)
        for tag,i,j,k,l in reversed(matcher.get_opcodes()):
            if tag=='equal':continue
            if tag=='replace' and j-i==l-k:
                for offset in range(j-i):operations.extend(difference(base[i+offset],value[k+offset],(*path,i+offset)))
            else:operations.append({'op':'splice','path':list(path),'start':i,'delete_count':j-i,'values':copy.deepcopy(value[k:l])})
        return operations
    return [{'op':'set','path':list(path),'value':copy.deepcopy(value)}]


def apply_difference(base, operations):
    result=copy.deepcopy(base)
    def reference(path):
        value=base
        for key in path:value=value[key]
        return copy.deepcopy(value)
    for op in operations:
        path=op['path']
        if not isinstance(path,list):raise ValueError('Invalid delta path')
        current=result
        for key in path[:-1]:current=current[key]
        if op['op']=='set':
            value=reference(op['value_from_base']) if 'value_from_base' in op else copy.deepcopy(op['value'])
            if path:current[path[-1]]=value
            else:result=value
        elif op['op']=='delete':
            if not path or not isinstance(current,dict):raise ValueError('Invalid delete')
            del current[path[-1]]
        elif op['op']=='splice':
            target=current[path[-1]] if path else result
            start,count=op['start'],op['delete_count']
            if not isinstance(target,list) or type(start) is not int or type(count) is not int or not 0<=start<=len(target) or not 0<=count<=len(target)-start:
                raise ValueError('Invalid splice')
            values=([reference(e['base_path']) if 'base_path' in e else copy.deepcopy(e['literal']) for e in op['entries']]
                    if 'entries' in op else copy.deepcopy(op['values']))
            target[start:start+count]=values
        else:raise ValueError('Unknown delta operation')
    return result


def bind_scopes(base, originals, semantic_key):
    if not semantic_key:raise ValueError('Explicit co-reading evidence required')
    scopes=[];owners={};names=set();references={}
    def index(value,path=()):
        if not isinstance(value,(dict,list)):return
        key=canonical(value)
        if len(key)>len(canonical(path))+32:references.setdefault(key,list(path))
        for k,v in (value.items() if isinstance(value,dict) else enumerate(value)):index(v,(*path,k))
    index(base)
    for original in originals:
        name=original['scope'];targets=original['targets'];request=original['request']
        if not isinstance(name,str) or not name or name in names:raise ValueError('Duplicate/invalid scope')
        names.add(name)
        if not targets or len(targets)!=len(set(targets)):raise ValueError('Invalid target ownership')
        for target in targets:
            if not isinstance(target,str) or target in owners:raise ValueError('Duplicate/invalid target owner')
            owners[target]=name
        delta=difference(base,request)
        for op in delta:
            if op['op']=='set' and canonical(op['value']) in references:
                op['value_from_base']=references[canonical(op.pop('value'))]
            elif op['op']=='splice':
                entries=[{'base_path':references[canonical(v)]} if canonical(v) in references else {'literal':v} for v in op['values']]
                if len(canonical(entries))<len(canonical(op['values'])):
                    op['entries']=entries;del op['values']
        if canonical(apply_difference(base,delta))!=canonical(request):raise ValueError('Scope inverse differs')
        scopes.append({'scope':name,'targets':list(targets),'request_fingerprint':digest(request),'delta':delta})
    return {'version':'co-read-scopes-v1','base_fingerprint':digest(base),'semantic_key':semantic_key,
            'scopes':scopes,'target_owners':owners}


def restore_scopes(base, manifest):
    if manifest['version']!='co-read-scopes-v1' or digest(base)!=manifest['base_fingerprint']:
        raise ValueError('Co-reading base changed')
    result={};owners={}
    for scope in manifest['scopes']:
        if scope['scope'] in result:raise ValueError('Duplicate scope')
        request=apply_difference(base,scope['delta'])
        if digest(request)!=scope['request_fingerprint']:raise ValueError('Original scope changed')
        for target in scope['targets']:
            if target in owners:raise ValueError('Duplicate owner')
            owners[target]=scope['scope']
        result[scope['scope']]=request
    if owners!=manifest['target_owners']:raise ValueError('Target ownership changed')
    return result


def split_candidate(base, manifest, translations):
    restore_scopes(base,manifest)
    if set(translations)!=set(manifest['target_owners']) or any(not isinstance(v,str) for v in translations.values()):
        raise ValueError('Joint candidate target mismatch')
    return {scope['scope']:{'translations':{key:translations[key] for key in scope['targets']},'approved':False}
            for scope in manifest['scopes']}
