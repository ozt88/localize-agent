"""Lossless pools and bounded overlap batching for independent complete scopes.

Adapters choose record paths and compatibility. Shared code never infers story
relationships or removes fields, and compiled jobs are not reviewed results.
"""
import copy
from collections import defaultdict
from .core import canonical, digest, optimize, measure_job, UTF8_METER

POOL_INSTRUCTIONS=(
    'For codec=scope-pool-v1, records is a shared exact-value table. Each scope has '
    'body and bindings. For each binding, insert the array of records at its indices '
    'into body at path, retaining index order and repeated indices. This restores '
    'that complete independent scope. Shared records do NOT imply story continuity, '
    'shared speakers, shared approval or shared term policies. Reason within each '
    'scope separately; preserve its conditions and alternatives. '
)
TASK=(
    'Translate every target in dynamic.target_contract into the specified target locale. '
    'For each target use ONLY its owning unit_id scope and that scope policy, '
    'terminology, speakers, source comments and all branches. Other scopes are '
    'independent even when records overlap. Do not translate context-only items. '
    'Preserve technical identifiers, placeholders and markup exactly. Return one '
    'unit per unit_id, with every assigned id exactly once. '
)


def at(value,path):
    for part in path:value=value[part]
    return value


def pool_scopes(scopes):
    """scopes: [{unit_id, data, record_paths}]. Only explicit array paths pooled."""
    records=[];indices={};units=[];names=set()
    for scope in scopes:
        name=scope['unit_id']
        if not isinstance(name,str) or not name or name in names:raise ValueError('Duplicate/invalid unit identity')
        names.add(name);body=copy.deepcopy(scope['data']);bindings=[];paths=[]
        for raw in scope.get('record_paths',[]):
            path=tuple(raw)
            if not path or not isinstance(path[-1],str):raise ValueError('Record path must address an object field')
            if any(path[:len(p)]==p or p[:len(path)]==path for p in paths):raise ValueError('Overlapping record paths')
            paths.append(path);values=at(scope['data'],path)
            if not isinstance(values,list):raise ValueError('Record path is not an array')
            refs=[]
            for value in values:
                # The full canonical value, not a game GUID, defines equality.
                key=canonical(value)
                if key not in indices:
                    indices[key]=len(records);records.append(copy.deepcopy(value))
                refs.append(indices[key])
            del at(body,path[:-1])[path[-1]]
            bindings.append({'path':list(path),'indices':refs})
        units.append({'unit_id':name,'body':body,'bindings':bindings})
    packet={'codec':'scope-pool-v1','records':records,'scopes':units}
    original={s['unit_id']:s['data'] for s in scopes}
    if canonical(unpool_scopes(packet))!=canonical(original):raise ValueError('Scope pool roundtrip differs')
    return packet


def unpool_scopes(packet):
    if packet.get('codec')!='scope-pool-v1' or not isinstance(packet.get('records'),list):raise ValueError('Invalid scope pool')
    restored={}
    for scope in packet['scopes']:
        name=scope['unit_id']
        if name in restored:raise ValueError('Duplicate pooled scope')
        body=copy.deepcopy(scope['body'])
        for binding in scope['bindings']:
            path=binding['path'];refs=binding['indices']
            if not path or not isinstance(path[-1],str):raise ValueError('Invalid pooled path')
            parent=at(body,path[:-1])
            if not isinstance(parent,dict) or path[-1] in parent:raise ValueError('Pooled binding overwrites data')
            if any(type(i) is not int or not 0<=i<len(packet['records']) for i in refs):raise ValueError('Invalid pooled reference')
            parent[path[-1]]=[copy.deepcopy(packet['records'][i]) for i in refs]
        restored[name]=body
    return restored


def compile_bundle(scopes,limits,meter=UTF8_METER,warmup='',task=TASK,budget_meter=UTF8_METER):
    """Complete independent request bodies. Does not change operational pipeline."""
    if not scopes:raise ValueError('Empty bundle')
    if len({canonical(s['compatibility']) for s in scopes})!=1:raise ValueError('Incompatible bundle policies')
    source={s['unit_id']:s['data'] for s in scopes}
    pool=pool_scopes(scopes);targets=[{'unit_id':s['unit_id'],**t} for s in scopes for t in s['targets']]
    if not targets or any(not s['targets'] for s in scopes):raise ValueError('Empty target scope')
    task=task+' Current bundle compatibility (project/locale/policy): '+canonical(scopes[0]['compatibility'])+'. '
    schema={'type':'object','properties':{'units':{'type':'array','items':{'type':'object','properties':{
        'unit_id':{'type':'string'},'translations':{'type':'array','items':{'type':'object','properties':{
            'id':{'type':'string'},'text':{'type':'string'}},'required':['id','text'],'additionalProperties':False}}},
        'required':['unit_id','translations'],'additionalProperties':False}}},'required':['units'],'additionalProperties':False}
    ceiling=max(0,min(limits.max_input,limits.context_window-limits.output_reserve)
                -budget_meter.count(warmup)-budget_meter.count(canonical(schema)))
    choice=optimize(source,task,meter,targets=targets,ceiling=ceiling,ceiling_meter=budget_meter,
                    alternatives=[('scope-pool-v1',pool,POOL_INSTRUCTIONS,unpool_scopes)])
    job={'prompt':choice['prompt'],'warmup':warmup,'schema':schema}
    audit=measure_job(job,len(targets),limits,budget_meter)
    scope_hashes={s['unit_id']:digest(s['data']) for s in scopes}
    if len(scopes)>limits.max_units:
        audit['allowed']=False;audit['reasons'].append('unit_budget_exceeded')
    return {'job':job,'audit':audit,'optimization':{k:v for k,v in choice.items() if k!='prompt'},
            'scope_sha256':scope_hashes,
            'targets':targets,'contract_sha256':digest({'targets':targets,'scopes':scope_hashes}),'approved':False}


def split_output(bundle,answer):
    """Exact ownership validation; successful splitting never grants approval."""
    if digest({'targets':bundle['targets'],'scopes':bundle['scope_sha256']})!=bundle['contract_sha256']:raise ValueError('Target contract changed')
    expected=defaultdict(set)
    for target in bundle['targets']:expected[target['unit_id']].add(target['id'])
    results={}
    for unit in answer['units']:
        name=unit['unit_id']
        if name in results or name not in expected:raise ValueError('Wrong/duplicate output scope')
        values={}
        for row in unit['translations']:
            if row['id'] in values or not isinstance(row['text'],str):raise ValueError('Duplicate/invalid output target')
            values[row['id']]=row['text']
        if set(values)!=expected[name]:raise ValueError('Output target ownership differs')
        results[name]={'translations':values,'approved':False,'source_sha256':bundle['scope_sha256'][name]}
    if set(results)!=set(expected):raise ValueError('Missing output scope')
    return results


def plan_overlap(scopes,evaluate,max_units=8,neighbors=8):
    """Greedy cost-decreasing merges of compatible, indivisible scopes.

    Only bounded high-overlap candidates are evaluated; no global-optimality
    claim. evaluate returns actual rendered multi-stage eligibility and cost.
    """
    if max_units<1 or neighbors<1:raise ValueError('Positive planning bounds required')
    names=[s['unit_id'] for s in scopes]
    if len(names)!=len(set(names)):raise ValueError('Duplicate planning scope')
    groups={i:[i] for i in range(len(scopes))};owner={i:i for i in groups};cache={};weights={};inverted=defaultdict(list)
    def score(indices):
        key=tuple(sorted(indices))
        if key not in cache:
            result=evaluate([scopes[i] for i in key])
            if type(result.get('allowed')) is not bool or type(result.get('cost')) is not int or result['cost']<0:
                raise ValueError('Invalid bundle evaluation')
            cache[key]=result
        return cache[key]
    for i,scope in enumerate(scopes):
        keys=set()
        for path in scope.get('record_paths',[]):
            for record in at(scope['data'],path):
                body=canonical(record);key=digest(record);weights[key]=len(body);keys.add(key)
        for key in keys:inverted[(canonical(scope['compatibility']),key)].append(i)
    candidates={}
    for i,scope in enumerate(scopes):
        overlap=defaultdict(int)
        keys={digest(r) for path in scope.get('record_paths',[]) for r in at(scope['data'],path)}
        for key in keys:
            for j in inverted[(canonical(scope['compatibility']),key)]:
                if i!=j:overlap[j]+=weights[key]
        for j,value in sorted(overlap.items(),key=lambda x:(-x[1],x[0]))[:neighbors]:
            candidates[tuple(sorted((i,j)))]=value
    for (i,j),_ in sorted(candidates.items(),key=lambda x:(-x[1],x[0])):
        a,b=owner[i],owner[j]
        if a==b or len(groups[a])+len(groups[b])>max_units:continue
        merged=sorted(groups[a]+groups[b]);result=score(merged)
        if not result['allowed']:continue
        old_a,old_b=score(groups[a]),score(groups[b])
        if old_a['allowed'] and old_b['allowed'] and result['cost']>=old_a['cost']+old_b['cost']:continue
        groups[a]=merged;del groups[b]
        for member in merged:owner[member]=a
    ready=[];held=[]
    for indices in groups.values():
        result=score(indices);entry={'indices':indices,'unit_ids':[names[i] for i in indices],'evaluation':result}
        (ready if result['allowed'] else held).append(entry)
    return {'ready':ready,'held':held,'evaluations':len(cache),'input_scopes':len(scopes),
            'output_scopes':sum(len(g['indices']) for g in ready+held),'method':'bounded greedy exact-record-overlap merges'}
