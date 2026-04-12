#!/usr/bin/env python3
"""
leiden.py — Leiden community detection for Vela knowledge graphs.

Reads a JSON object from stdin:
  {"nodes": ["id1", "id2", ...], "edges": [{"from": "id1", "to": "id2"}, ...]}

Writes a JSON object to stdout:
  {"id1": 0, "id2": 0, "id3": 1, ...}   (node ID → community ID, 0-indexed)

Requires: graspologic (pip install graspologic)
Falls back to a simple connected-components partition if graspologic is missing.
"""

import json
import sys


def leiden_partition(nodes, edges):
    """Run Leiden community detection via graspologic."""
    import numpy as np
    from graspologic.partition import leiden

    n = len(nodes)
    if n == 0:
        return {}

    idx = {node: i for i, node in enumerate(nodes)}
    adj = np.zeros((n, n), dtype=float)

    for e in edges:
        f = idx.get(e.get("from", ""))
        t = idx.get(e.get("to", ""))
        if f is not None and t is not None:
            adj[f][t] = 1.0
            adj[t][f] = 1.0  # treat as undirected for clustering

    labels = leiden(adj)
    # labels is a list of community IDs aligned to the node index
    return {node: int(labels[i]) for i, node in enumerate(nodes)}


def fallback_components(nodes, edges):
    """Simple union-find connected components — used when graspologic is absent."""
    parent = {n: n for n in nodes}

    def find(x):
        while parent[x] != x:
            parent[x] = parent[parent[x]]
            x = parent[x]
        return x

    def union(a, b):
        ra, rb = find(a), find(b)
        if ra != rb:
            parent[ra] = rb

    for e in edges:
        f, t = e.get("from", ""), e.get("to", "")
        if f in parent and t in parent:
            union(f, t)

    # Map component root → integer community ID
    roots = {}
    result = {}
    for node in nodes:
        root = find(node)
        if root not in roots:
            roots[root] = len(roots)
        result[node] = roots[root]
    return result


def main():
    data = json.load(sys.stdin)
    nodes = data.get("nodes", [])
    edges = data.get("edges", [])

    try:
        partition = leiden_partition(nodes, edges)
    except Exception:
        # graspologic not available or failed — use fallback
        partition = fallback_components(nodes, edges)

    json.dump(partition, sys.stdout)


if __name__ == "__main__":
    main()
