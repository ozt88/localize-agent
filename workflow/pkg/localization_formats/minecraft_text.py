"""Observed Minecraft/FTB text contracts. Never repairs technical data by guessing."""
from collections import Counter
import json
import re

COLOR = re.compile(r'(?<!\\)(?:&|§)(?:#[0-9a-fA-F]{6}|[0-9a-zA-Z])')
MARKER = re.compile(r'\{(?:image[^{}]*|@pagebreak)\}')
FORMAT = re.compile(r'(?<!\d)%(?:\d+\$)?[-#+0,(]*\d*(?:\.\d+)?[sdfbcxXoegGaAtThHn%]')
RESOURCE = re.compile(r'\b[a-z][a-z0-9_]*:[a-z0-9_./-]+')
URL = re.compile(r'https?://[^\s<>"\]}]+')


def component(text):
    s = text.lstrip()
    if re.match(r'(?:\[\s*["{]|\{\s*")', s):
        value = json.loads(s)
        if not isinstance(value, (list, dict)):
            raise ValueError('Expected a text component')
        return value
    return None


def text_paths(value, path=()):
    """Only text nodes and show_text hover contents are mutable."""
    if isinstance(value, str):
        yield path
    elif isinstance(value, list):
        for n, v in enumerate(value):
            yield from text_paths(v, path + (n,))
    elif isinstance(value, dict):
        if isinstance(value.get('text'), str):
            yield path + ('text',)
        if 'extra' in value:
            yield from text_paths(value['extra'], path + ('extra',))
        hover = value.get('hoverEvent')
        if isinstance(hover, dict) and hover.get('action') == 'show_text' and 'contents' in hover:
            yield from text_paths(hover['contents'], path + ('hoverEvent', 'contents'))


def value_at(value, path):
    for part in path: value = value[part]
    return value


def structural_signature(value):
    allowed = set(text_paths(value))
    def walk(v, path):
        if path in allowed: return '<VISIBLE_TEXT>'
        if isinstance(v, list): return [walk(x, path + (i,)) for i, x in enumerate(v)]
        if isinstance(v, dict): return {k: walk(x, path + (k,)) for k, x in v.items()}
        return v
    return walk(value, ())


def technical_only(text):
    return bool(text.strip()) and not MARKER.sub('', text).strip()


def plain_failures(source, target):
    errors = []
    for label, pattern, ordered in [('color', COLOR, True), ('marker', MARKER, True),
                                    ('format', FORMAT, True), ('resource', RESOURCE, False),
                                    ('url', URL, False)]:
        a, b = pattern.findall(source), pattern.findall(target)
        if label == 'resource':
            # A sentence-final dot is punctuation, not part of a displayed ID/coordinate.
            a, b = [s.rstrip('.') for s in a], [s.rstrip('.') for s in b]
        if (a != b if ordered else Counter(a) != Counter(b)):
            errors.append(label + '_changed')
    if source.count('\\&\\') != target.count('\\&\\'):
        errors.append('escaped_ampersand_changed')
    if Counter(re.findall(r'(?<![\w:])/\w+', source)) != Counter(re.findall(r'(?<![\w:])/\w+', target)):
        errors.append('command_name_changed')
    if Counter(re.findall(r'<[a-zA-Z_][^<>]*>', source)) != Counter(re.findall(r'<[a-zA-Z_][^<>]*>', target)):
        errors.append('angle_placeholder_changed')
    return errors


def validate(source, target):
    if not isinstance(target, str) or (source.strip() and not target.strip()):
        return ['empty_translation']
    try:
        a, b = component(source), component(target)
        if a is None:
            return ['component_added'] if b is not None else plain_failures(source, target)
        if b is None or structural_signature(a) != structural_signature(b):
            return ['component_structure_changed']
        errors = []
        for path in text_paths(a):
            errors.extend(plain_failures(value_at(a, path), value_at(b, path)))
        return sorted(set(errors))
    except (ValueError, TypeError, KeyError, IndexError):
        return ['invalid_component']
