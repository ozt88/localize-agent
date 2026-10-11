"""Public, engine-independent input planning and explicitly experimental selection."""
from .core import (Limits, Meter, UTF8_METER, canonical, digest, pack, unpack,
                   optimize, render_prompt, measure_job, plan, project_results)
from .scopes import bind_scopes, restore_scopes, split_candidate, difference, apply_difference
from .grouping import partition_co_read
from .compact_codec import compact as compact_records, unpack_compact
from .bundles import pool_scopes, unpool_scopes, compile_bundle, plan_overlap, split_output
from .evidence_selection import select as select_evidence, restore as restore_evidence, expand as expand_evidence
from .cost_accounting import aggregate as aggregate_usage
