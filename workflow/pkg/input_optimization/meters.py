"""Optional tokenizer used for ranking, never silently an exact provider count."""
from functools import lru_cache
from .core import Meter


@lru_cache(maxsize=4)
def token_meter(encoding='o200k_base'):
    import tiktoken
    tokenizer=tiktoken.get_encoding(encoding)
    return Meter(encoding,lambda text:len(tokenizer.encode(text,disallowed_special=())), 'estimate')
