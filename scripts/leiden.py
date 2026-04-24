#!/usr/bin/env python3
"""
leiden.py — Leiden community detection for Vela knowledge graphs.

Reads a JSON object from stdin:
  {"nodes": ["id1", "id2", ...], "edges": [{"from": "id1", "to": "id2"}, ...]}

Writes a JSON object to stdout:
  {"id1": 0, "id2": 0, "id3": 1, ...}   (node ID → community ID, 0-indexed)

Requires either:
  - graspologic (preferred Leiden backend)
  - networkx (fallback modularity backend)
"""

import json
import sys


def leiden_partition(nodes, edges):
    """Run Leiden community detection via graspologic."""
    try:
        from graspologic.partition import leiden
    except ModuleNotFoundError as exc:
        raise RuntimeError("graspologic is not installed") from exc

    import numpy as np

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


def networkx_partition(nodes, edges):
    """Run modularity-based community detection via NetworkX."""
    try:
        import networkx as nx
    except ModuleNotFoundError as exc:
        raise RuntimeError(
            "networkx is not installed; install it with: .venv/bin/pip install -r requirements-clustering.txt"
        ) from exc

    if not nodes:
        return {}

    graph = nx.Graph()
    graph.add_nodes_from(nodes)
    node_set = set(nodes)
    graph.add_edges_from(
        (edge.get("from", ""), edge.get("to", ""))
        for edge in edges
        if edge.get("from", "") in node_set and edge.get("to", "") in node_set
    )

    if graph.number_of_edges() == 0:
        return {node: i for i, node in enumerate(nodes)}

    try:
        communities = nx.community.louvain_communities(graph, seed=0)
    except AttributeError:
        communities = nx.community.greedy_modularity_communities(graph)

    partition = {}
    for idx, community in enumerate(communities):
        for node in community:
            partition[node] = idx
    for node in nodes:
        if node not in partition:
            partition[node] = len(partition)
    return partition

def main():
    data = json.load(sys.stdin)
    nodes = data.get("nodes", [])
    edges = data.get("edges", [])

    try:
        partition = leiden_partition(nodes, edges)
    except Exception as exc:
        if "graspologic is not installed" not in str(exc):
            print(str(exc), file=sys.stderr)
            sys.exit(1)
        try:
            partition = networkx_partition(nodes, edges)
        except Exception as fallback_exc:
            print(f"{exc}; {fallback_exc}", file=sys.stderr)
            sys.exit(1)

    json.dump(partition, sys.stdout)


if __name__ == "__main__":
    main()
