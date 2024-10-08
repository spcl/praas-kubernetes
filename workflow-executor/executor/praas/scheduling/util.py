from typing import Set, Dict, List, Iterable

from executor.models import Workflow, mem_to_bytes
from .types import Reservation


def get_stages(workflow: Workflow):
    nodes = get_ids(workflow)
    out_edges = get_out_edges(workflow)
    in_edges = get_in_edges(workflow)

    stage_candidates, mapping = components(nodes, out_edges)
    stage_dependencies = dependencies(stage_candidates, in_edges, mapping)
    return components_to_stages(stage_candidates, stage_dependencies)


def get_ids(workflow: Workflow) -> Set[int]:
    ids = set()
    for instances in workflow.functions.values():
        ids.update(instances)
    return ids


def get_out_edges(workflow: Workflow) -> Dict[int, Set[int]]:
    out_edges = dict()
    for src, edges in workflow.communications.items():
        out_edges[src] = set()
        for edge in edges:
            out_edges[src].add(edge.dst)
    return out_edges


def get_in_edges(workflow: Workflow) -> Dict[int, Set[int]]:
    in_edges = dict()
    for src, edges in workflow.communications.items():
        for edge in edges:
            if edge.dst not in in_edges:
                in_edges[edge.dst] = set()
            in_edges[edge.dst].add(src)
    return in_edges


def components_to_stages(ssc, deps):
    stages = []

    while len(deps) != 0:
        # Find all the components that do not have any dependencies
        curr_stage = set()
        curr_stage_sscs = set()
        for ssc_id, dep in deps.items():
            if len(dep) == 0:
                curr_stage_sscs.add(ssc_id)
                for n in ssc[ssc_id]:
                    curr_stage.add(n)
        stages.append(curr_stage)

        # Remove curr stage elements from the dependencies
        for ssc_id in curr_stage_sscs:
            del deps[ssc_id]

        # Remove dependencies from this stage
        for ssc_id, dep in deps.items():
            deps[ssc_id] = dep.difference(curr_stage_sscs)

    return stages


def dependencies(ssc, in_edges, mapping):
    derived_in_edges = dict()

    for ssc_idx, component in ssc.items():
        ancestor_comps = set()
        for node in component:
            if node in in_edges:
                for edge in in_edges[node]:
                    in_ssc = mapping[edge]
                    if in_ssc != ssc_idx:
                        ancestor_comps.add(in_ssc)
        derived_in_edges[ssc_idx] = ancestor_comps
    return derived_in_edges


def components(nodes: Set[int], out_edges: Dict[int, Iterable[int]]):
    # Tarjan's strongly connected components algorithm
    index = 0
    stack = []
    ssc = dict()
    indexes = dict()
    on_stack = dict()
    low_links = dict()
    node_to_ssc = dict()

    for n in nodes:
        if n not in indexes:
            strong_connect(n, index, stack, indexes, low_links, on_stack, out_edges, ssc, node_to_ssc)

    return ssc, node_to_ssc


def strong_connect(v: int, index: int, stack: List[int], indexes: Dict[int, int], low_links: Dict[int, int],
                   on_stack: Dict[int, bool], out_edges: Dict[int, Iterable[int]], ssc: Dict[int, Set[int]],
                   node_to_ssc: Dict[int, int]):
    indexes[v] = index
    low_links[v] = index
    index += 1

    stack.append(v)
    on_stack[v] = True

    if v in out_edges:
        for dst in out_edges[v]:
            if dst not in indexes:
                strong_connect(dst, index, stack, indexes, low_links, on_stack, out_edges, ssc, node_to_ssc)
                low_links[v] = min(low_links[v], low_links[dst])
            elif dst in on_stack and on_stack[dst]:
                low_links[v] = min(low_links[v], indexes[dst])

    if low_links[v] == indexes[v]:
        popped_all = False
        c = set()
        ssc_idx = len(ssc) + 1
        while not popped_all:
            w = stack.pop()
            on_stack[w] = False
            popped_all = w == v
            c.add(w)
            node_to_ssc[w] = ssc_idx
        ssc[ssc_idx] = c


def get_requirements(workflow: Workflow) -> Dict[int, Reservation]:
    res = dict()
    for req in workflow.requirements:
        curr_res = Reservation(cpu=req.cpu, mem=mem_to_bytes(req.mem))
        for instance in req.instances:
            res[instance] = curr_res

    return res
