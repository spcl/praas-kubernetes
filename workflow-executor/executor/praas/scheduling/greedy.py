import copy
from operator import itemgetter
from typing import List, Set

from . import util
from .types import Schedule, Reservation
from executor.models import Workflow


def merge(workflow: Workflow, schedule: Schedule) -> None:
    """ Approximation algorithm based on merging along the highest valued edge """
    requirements = util.get_requirements(workflow)
    stages = util.get_stages(workflow)
    instances = util.get_ids(workflow)

    # First find all the stages
    job_to_stage = dict()
    for idx, stage in enumerate(stages):
        for func in stage:
            job_to_stage[func] = idx

    # Allocate each function on its own node
    reservations = dict()
    node_to_comp = dict()
    comp_to_nodes = dict()
    for func_inst in instances:
        reservations[func_inst] = {job_to_stage[func_inst]: requirements[func_inst]}
        node_to_comp[func_inst] = func_inst
        comp_to_nodes[func_inst] = {func_inst}

    # Construct all objects that track edges
    in_edges, out_edges, edge_weights = construct_graph(workflow, stages)

    # Merge along the highest weighted edges
    did_merge = True
    while did_merge:
        did_merge = False

        # Sort edges
        sorted_edges = edge_weights.keys()
        sorted_edges = sorted(sorted_edges,
                              key=lambda e: get_degree_sum(in_edges, out_edges, e),
                              reverse=False
                              )
        sorted_edges = sorted(sorted_edges, key=lambda x: edge_weights[x], reverse=True)

        # Find the highest weighted edge we can merge
        idx = 0
        while (not did_merge) and idx < len(sorted_edges):
            src, dst = sorted_edges[idx]
            idx += 1
            # Can we merge?
            can_merge = True
            for stage in range(len(stages)):
                reservation_src = reservations[src].get(stage, Reservation())
                reservation_dst = reservations[dst].get(stage, Reservation())
                new_res = reservation_src + reservation_dst
                can_merge = can_merge and new_res.fits(cpu=schedule.cpu_limit, mem=schedule.mem_limit)

            if can_merge:
                # Merge
                did_merge = True
                update_edges(src, dst, edge_weights, in_edges, out_edges)
                comp_to_nodes[src].update(comp_to_nodes[dst])
                for node in comp_to_nodes[dst]:
                    node_to_comp[node] = src
                # Update reservations
                for stage, res in reservations[dst].items():
                    reservation = reservations[src].get(stage, Reservation())
                    reservations[src][stage] = reservation + res
                del comp_to_nodes[dst]
                del reservations[dst]

        # Check if we are still going to merge and if no check number of processes for each stage
        if not did_merge and len(reservations.keys()) > schedule.process_limit:
            proc_per_stage = dict()
            # Find stages with too many processes
            for proc, res in reservations.items():
                for stage in res.keys():
                    if stage in proc_per_stage:
                        proc_per_stage[stage].add(proc)
                    else:
                        proc_per_stage[stage] = {proc}

            created_stages = 0
            prev_stage_processes = set()
            for stage_idx, processes in sorted(proc_per_stage.items(), key=itemgetter(0)):
                if len(processes) <= schedule.process_limit:
                    prev_stage_processes.update(processes)
                if len(processes) > schedule.process_limit:
                    # This is a stage that has more processes than the previous
                    # Find all processes and filter those out that occur in the previous stage as well
                    did_merge = True
                    adjusted_stage_idx = stage_idx + created_stages
                    movable = processes.difference(prev_stage_processes)
                    movable = sorted(movable, key=lambda m: sum_edges(m, in_edges, out_edges, edge_weights),
                                     reverse=True)
                    if len(movable) == 0:
                        continue

                    num_to_move = min(len(processes) - schedule.process_limit, len(movable))
                    for proc in movable[num_to_move:]:
                        prev_stage_processes.add(proc)
                    movable = movable[:num_to_move]
                    old_stage = stages[adjusted_stage_idx]
                    new_stage = set()
                    stages.insert(adjusted_stage_idx + 1, new_stage)
                    created_stages += 1
                    for proc, res in reservations.items():
                        sorted_stages = sorted(res.keys(), reverse=True)
                        for stage in sorted_stages:
                            if stage > adjusted_stage_idx:
                                res[stage + 1] = copy.copy(res[stage])
                                del res[stage]
                    for proc in movable:
                        reservations[proc][adjusted_stage_idx + 1] = reservations[proc][adjusted_stage_idx]
                        del reservations[proc][adjusted_stage_idx]
                        for node in comp_to_nodes[proc]:
                            if job_to_stage[node] == stage_idx:
                                old_stage.remove(node)
                                new_stage.add(node)
                                job_to_stage[node] = adjusted_stage_idx + 1
            if not did_merge:
                did_merge = True
                # Add 0 weight edges between all components
                comps = reservations.keys()
                for comp1 in comps:
                    for comp2 in comps:
                        if comp1 == comp2 or (comp1, comp2) in edge_weights or (comp2, comp1) in edge_weights:
                            continue
                        key = (comp1, comp2)
                        edge_weights[key] = 0

                        out_edges[comp1].add(comp2)
                        in_edges[comp2].add(comp1)

    # And now schedule based on components
    for stage in stages:
        for job in stage:
            schedule.add_function(func_instance=job, process=node_to_comp[job], req=requirements[job])
        schedule.next_stage()


def get_degree_sum(in_edges, out_edges, edge):
    deg = 0
    deg += len(in_edges.get(edge[0], set()))
    deg += len(out_edges.get(edge[0], set()))
    deg += len(in_edges.get(edge[1], set()))
    deg += len(out_edges.get(edge[1], set()))
    return deg


def update_edges(src, dst, edge_weights, in_edges, out_edges):
    # Update incoming edges
    # First edges coming from other components
    if dst in in_edges:
        for in_edge in in_edges[dst]:
            # Repoint them to the new component
            out_edges[in_edge].remove(dst)
            out_edges[in_edge].add(src)

            # Update weight
            if in_edge != src:
                old_weight = edge_weights[(in_edge, dst)]
                if (in_edge, src) in edge_weights:
                    edge_weights[(in_edge, src)] += old_weight
                else:
                    edge_weights[(in_edge, src)] = old_weight
            del edge_weights[in_edge, dst]
        if src in in_edges:
            in_edges[src].update(copy.copy(in_edges[dst]))
        else:
            in_edges[src] = copy.copy(in_edges[dst])
        del in_edges[dst]

    if dst in out_edges:
        for out_edge in out_edges[dst]:
            # Update in edges of other components
            if out_edge in in_edges:
                in_edges[out_edge].remove(dst)
                in_edges[out_edge].add(src)

            # Update edge weights
            if out_edge != src:
                old_weight = edge_weights[(dst, out_edge)]
                if (src, out_edge) in edge_weights:
                    edge_weights[(src, out_edge)] += old_weight
                else:
                    edge_weights[(src, out_edge)] = old_weight
            del edge_weights[(dst, out_edge)]
        if src in out_edges:
            out_edges[src].update(copy.copy(out_edges[dst]))
        else:
            out_edges[src] = copy.copy(out_edges[dst])
        del out_edges[dst]

    if src in out_edges:
        out_edges[src].discard(src)
    if src in in_edges:
        in_edges[src].discard(src)


def sum_edges(process, in_edges, out_edges, weights):
    val = 0
    if process in in_edges:
        for src in in_edges[process]:
            val += weights[(src, process)]
    if process in out_edges:
        for dst in out_edges[process]:
            val += weights[(process, dst)]
    return val


def construct_graph(workflow: Workflow, stages: List[Set[int]]):
    # First with proper edges
    in_edges = util.get_in_edges(workflow)
    out_edges = util.get_out_edges(workflow)
    edge_weights = dict()
    for src, edges in workflow.communications.items():
        for edge in edges:
            key = (src, edge.dst)
            edge_weights[key] = edge.weight

    # Now add the edges inside each stage
    for stage in stages:
        for n1 in stage:
            for n2 in stage:
                if n2 <= n1:
                    continue

                key = (n1, n2)
                edge_weights[key] = 0

                if n2 not in in_edges:
                    in_edges[n2] = set()
                in_edges[n2].add(n1)

                if n1 not in out_edges:
                    out_edges[n1] = set()
                out_edges[n1].add(n2)

    return in_edges, out_edges, edge_weights

