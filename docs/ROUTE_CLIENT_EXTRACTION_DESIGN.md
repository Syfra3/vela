# Future route/client extraction design note

This iteration intentionally does **not** extract HTTP routes or API-client calls.
`vela_rank`, `vela_hotspots`, and `vela_module_summary` report graph metrics from
the currently indexed nodes/edges only and list route/client extraction as a gap.

Future implementation should add extractor stages that:

- identify server route declarations with method, path, handler symbol, owning module, and source range;
- identify API-client calls with method/path/template, caller symbol/file, and confidence;
- normalize both sides through a stable interface node so cross-app consumers can be counted;
- stamp evidence metadata with extractor name/version, confidence, source artifact, and freshness;
- expose cross-package/app consumer counts as available metrics rather than inferred zeroes.

Until those extractors exist, compact ranking tools must keep optional route/client
and cross-app/package metrics explicit as `unavailable`.
