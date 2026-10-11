"""Compact, reversible spelling of record-table-v1; no game-specific fields."""
import copy

INSTRUCTIONS = (
    'For codec=record-table-v2, decode root with schemas, strings and shared. '
    'An array starting with a negative integer -N-1 is record schema N: copy '
    'its literal constants and assign remaining values to fields in order. '
    'Other arrays retain order. {"array":[...]} escapes a literal array whose '
    'first value is a negative integer. Strings @sN and @dN reference strings[N] '
    'and shared[N]; {"literal":text} escapes literal text starting with @. '
    'Shared entries reference only earlier entries. Schema constants and the '
    'strings table are literal, not encoded. All fields, types and occurrences '
    'are preserved. Decode this outer table before any inner adapter codec. '
)


def compact(packet):
    from .core import canonical, unpack
    if packet.get('codec') != 'record-table-v1':
        raise ValueError('Expected record-table-v1')
    def encode(v):
        if isinstance(v, dict):
            if set(v)=={'$r'}:
                n,*values=v['$r']
                return [-n-1]+[encode(x) for x in values]
            if set(v)=={'$s'}:return '@s'+str(v['$s'])
            if set(v)=={'$d'}:return '@d'+str(v['$d'])
            raise ValueError('Unknown record-table tag')
        if isinstance(v,list):
            values=[encode(x) for x in v]
            return {'array':values} if v and type(v[0]) is int and v[0]<0 else values
        if isinstance(v,str) and v.startswith('@'):return {'literal':v}
        return v
    result={'codec':'record-table-v2','schemas':copy.deepcopy(packet['schemas']),
            'strings':copy.deepcopy(packet['strings']),
            'shared':[encode(v) for v in packet['shared']], 'root':encode(packet['root'])}
    if canonical(unpack(expand(result)))!=canonical(unpack(packet)):
        raise ValueError('Compact codec roundtrip differs')
    return result


def expand(packet):
    if not isinstance(packet,dict) or packet.get('codec')!='record-table-v2' or set(packet)!={'codec','schemas','strings','shared','root'}:
        raise ValueError('Invalid compact codec packet')
    if not all(isinstance(packet[k],list) for k in ('schemas','strings','shared')):
        raise ValueError('Invalid compact tables')
    def decode(v):
        if isinstance(v,dict):
            if set(v)=={'literal'} and isinstance(v['literal'],str) and v['literal'].startswith('@'):
                return v['literal']
            if set(v)=={'array'} and isinstance(v['array'],list):
                if not v['array'] or type(v['array'][0]) is not int or v['array'][0]>=0:
                    raise ValueError('Invalid array escape')
                return [decode(x) for x in v['array']]
            raise ValueError('Invalid compact escape')
        if isinstance(v,list):
            if v and type(v[0]) is int and v[0]<0:
                return {'$r':[-v[0]-1]+[decode(x) for x in v[1:]]}
            return [decode(x) for x in v]
        if isinstance(v,str) and v.startswith('@'):
            tag=v[1:2];number=v[2:]
            if tag not in ('s','d') or not number.isascii() or not number.isdigit():
                raise ValueError('Invalid compact reference')
            return {'$'+tag:int(number)}
        return v
    return {'codec':'record-table-v1','schemas':copy.deepcopy(packet['schemas']),
            'strings':copy.deepcopy(packet['strings']),
            'shared':[decode(v) for v in packet['shared']], 'root':decode(packet['root'])}


def unpack_compact(packet):
    from .core import unpack
    return unpack(expand(packet))
