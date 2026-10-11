"""JSON-in/JSON-out command usable from Go, Python or other project adapters."""
import argparse
import json
import sys
from .core import Limits, plan, measure_job
from .bundles import compile_bundle, plan_overlap
from .evidence_selection import select,restore,expand
from .cost_accounting import aggregate


def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('operation',choices=['plan','check-job','compile-bundle','plan-bundles',
        'select-evidence','restore-evidence','expand-evidence','account-usage'])
    args=parser.parse_args()
    data=json.load(sys.stdin)
    limits=Limits(**data['limits']) if 'limits' in data else None
    if args.operation in ('plan','check-job','compile-bundle','plan-bundles') and limits is None:
        raise ValueError('Input limits required')
    if args.operation=='select-evidence':
        view,archive=select(data['source'],data['deferrals'],data.get('required_paths',()),data.get('unresolved',()))
        result={'view':view,'archive':archive,'approved':False}
    elif args.operation=='restore-evidence':
        result=restore(data['view'],data['archive'])
    elif args.operation=='expand-evidence':
        view,archive=expand(data['view'],data['archive'],data['paths'])
        result={'view':view,'archive':archive,'approved':False}
    elif args.operation=='account-usage':
        result=aggregate(data['snapshots'],data.get('expected_sessions',()))
    elif args.operation=='plan':
        result=plan(data['units'],data['contexts'],limits,warmup=data.get('warmup',''),schema=data.get('schema'))
    elif args.operation=='compile-bundle':
        result=compile_bundle(data['scopes'],limits,warmup=data.get('warmup',''))
    elif args.operation=='plan-bundles':
        def evaluate(scopes):
            compiled=compile_bundle(scopes,limits,warmup=data.get('warmup',''))
            return {'allowed':compiled['audit']['allowed'],'cost':compiled['audit']['input_cost'],
                    'reasons':compiled['audit']['reasons']}
        result=plan_overlap(data['scopes'],evaluate,max_units=limits.max_units,neighbors=data.get('neighbors',8))
        result['cost_scope']='translation input only; independent review and registration are separate'
        result['approved']=False
        for group in result['ready']:
            group['compiled']=compile_bundle([data['scopes'][i] for i in group['indices']],limits,warmup=data.get('warmup',''))
    else:
        result=measure_job(data['job'],data['target_count'],limits)
    print(json.dumps(result,ensure_ascii=False,allow_nan=False))
    if args.operation=='check-job' and not result['allowed']:return 2
    return 0


if __name__=='__main__':
    try:sys.exit(main())
    except (ValueError,KeyError,TypeError) as exc:
        print(str(exc),file=sys.stderr);sys.exit(2)
