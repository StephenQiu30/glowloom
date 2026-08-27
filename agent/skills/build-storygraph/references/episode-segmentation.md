# Episode segmentation

Treat every `marker_hints` entry as an authoritative boundary start. Return boundaries in source order with sequential `episode_order`: the first starts at `0`, adjacent boundaries are contiguous, and the final boundary ends at `source_code_points`.

Copy boundary Evidence only from `marker_hints` or `evidence_index`. Every explicit marker must start a boundary and its exact Evidence must be present on that boundary. Use concise evidence-backed titles and report ambiguity in `review_issues`; never invent source positions, override a marker, or create Episode records.
