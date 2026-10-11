"""Retired entry point: historical sentence reuse is prohibited for this project."""


def prepare(*args, **kwargs):
    raise ValueError(
        'Historical sentence mapping is disabled by user policy. Translate from current '
        'English and current pack context. Historical terminology needs explicit '
        'term-level validation; matching keys or source text do not authorize sentence reuse.'
    )


if __name__ == '__main__':
    prepare()
