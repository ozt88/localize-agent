"""Budget-aware dynamic partitioning of adapter-defined co-reading families."""
from collections import OrderedDict


def partition_co_read(scopes, family_key, evaluate, max_scopes=8):
    """Minimize held scopes, then estimated cost, then calls within each family.

    family_key declares semantic AND policy compatibility; this module never
    infers it from similar text. evaluate returns allowed, nonnegative cost and
    optional artifact metadata after complete-context input/output validation.
    Scopes remain indivisible. DP optimizes contiguous partitions in original
    family order; it does not claim globally optimal arbitrary set partitioning.
    """
    if type(max_scopes) is not int or max_scopes<1:raise ValueError('Invalid group limit')
    families=OrderedDict()
    for index,scope in enumerate(scopes):families.setdefault(family_key(scope),[]).append((index,scope))
    ready=[];held=[]
    for key,members in families.items():
        n=len(members);best=[None]*(n+1);best[n]=((0,0,0),[])
        for start in range(n-1,-1,-1):
            tail,actions=best[start+1]
            options=[((tail[0]+1,tail[1],tail[2]),[{'held':members[start][0]}]+actions)]
            for end in range(start+1,min(n,start+max_scopes)+1):
                result=evaluate([scope for _,scope in members[start:end]])
                if not isinstance(result.get('allowed'),bool):raise ValueError('Evaluation must declare eligibility')
                if not result['allowed']:continue
                cost=result['cost']
                if type(cost) is not int or cost<0:raise ValueError('Invalid grouping cost')
                tail,actions=best[end]
                options.append(((tail[0],tail[1]+cost,tail[2]+1),
                                [{'indices':[i for i,_ in members[start:end]],'evaluation':result}]+actions))
            best[start]=min(options,key=lambda x:x[0])
        for action in best[0][1]:
            if 'held' in action:held.append({'index':action['held'],'scope':scopes[action['held']],'reason':'no_budget_valid_partition'})
            else:ready.append(action)
    covered=[i for group in ready for i in group['indices']]+[h['index'] for h in held]
    if sorted(covered)!=list(range(len(scopes))):raise ValueError('Partition coverage differs')
    return {'ready':ready,'held':held,'input_scopes':len(scopes),'output_scopes':len(covered)}
