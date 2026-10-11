"""Lossless text-span reader for observed FTB SNBT compounds/lists.

Not a general NBT type codec. Bare atoms are opaque; unsupported escapes/types
fail closed. Translation edits replace only explicitly addressed quoted leaves.
"""
import json
import re


class Document:
    def __init__(self, text):
        self.text, self.pos, self.spans = text, 0, {}
        self.value = self.read(())
        self.space()
        if self.pos != len(text): raise ValueError('Trailing SNBT content')

    def space(self):
        while self.pos < len(self.text):
            if self.text[self.pos].isspace() or self.text[self.pos] == ',': self.pos += 1
            elif self.text.startswith('//', self.pos) or self.text[self.pos] == '#':
                end = self.text.find('\n', self.pos)
                self.pos = len(self.text) if end < 0 else end + 1
            else: break

    def quoted(self):
        start = self.pos
        quote = self.text[self.pos]; self.pos += 1
        while self.pos < len(self.text):
            char = self.text[self.pos]; self.pos += 1
            if char == '\\': self.pos += 1
            elif char == quote:
                raw = self.text[start:self.pos]
                if quote != '"': raise ValueError('Single-quoted strings need an explicit codec')
                return json.loads(raw), (start, self.pos)
        raise ValueError('Unterminated SNBT string')

    def read(self, path):
        self.space()
        if self.pos >= len(self.text): raise ValueError('Unexpected SNBT end')
        char = self.text[self.pos]
        if char == '"':
            value, span = self.quoted(); self.spans[path] = span
            return value
        if char == '{':
            self.pos += 1; result = {}
            while True:
                self.space()
                if self.pos >= len(self.text): raise ValueError('Unclosed compound')
                if self.text[self.pos] == '}': self.pos += 1; return result
                if self.text[self.pos] == '"': key, _ = self.quoted()
                else:
                    match = re.match(r'[^\s:{}\[\],]+', self.text[self.pos:])
                    if not match: raise ValueError('Invalid key')
                    key = match[0]; self.pos += len(key)
                self.space()
                if self.pos >= len(self.text) or self.text[self.pos] != ':': raise ValueError('Missing colon')
                self.pos += 1
                if key in result: raise ValueError('Duplicate key: ' + key)
                result[key] = self.read(path + (key,))
        if char == '[':
            self.pos += 1; result = []
            typed = re.match(r'\s*([BIL]);', self.text[self.pos:])
            if typed: self.pos += len(typed[0])
            while True:
                self.space()
                if self.pos >= len(self.text): raise ValueError('Unclosed list')
                if self.text[self.pos] == ']':
                    self.pos += 1
                    return {'__snbt_array_type__': typed[1], 'values': result} if typed else result
                result.append(self.read(path + (len(result),)))
        match = re.match(r'[^\s,\[\]{}]+', self.text[self.pos:])
        if not match: raise ValueError('Unsupported SNBT token')
        self.pos += len(match[0])
        return match[0]

    def replace(self, replacements):
        edits = []
        for path, value in replacements.items():
            if path not in self.spans or not isinstance(value, str): raise ValueError('Not a quoted text leaf')
            start, end = self.spans[path]
            edits.append((start, end, json.dumps(value, ensure_ascii=False)))
        text = self.text
        for start, end, value in sorted(edits, reverse=True): text = text[:start] + value + text[end:]
        parsed = Document(text)
        for path, value in replacements.items():
            actual = parsed.value
            for part in path: actual = actual[part]
            if actual != value: raise ValueError('Replacement round trip failed')
        return text
