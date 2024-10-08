from collections import namedtuple
from typing import List, Set, Dict

from pydantic import BaseModel

from executor.models import Workflow


class Reservation:
    cpu: int
    mem: int

    def __init__(self, cpu: int = 0, mem: int = 0):
        self.cpu = cpu
        self.mem = mem

    def __add__(self, other):
        return Reservation(cpu=self.cpu + other.cpu, mem=self.mem + other.mem)

    def fits(self, cpu: int, mem: int):
        return self.cpu <= cpu and self.mem <= mem


class Schedule:
    FunctionAllocation = namedtuple("FunctionAllocation", "proc id cpu mem")
    processes: set
    mem_limit: int
    cpu_limit: int
    process_limit: int
    curr_stage_idx: int
    function_locations: Dict[int, int]
    stages: List[Set[FunctionAllocation]]
    curr_processes: Dict[int, Reservation]

    def __init__(self, process_limit, mem_limit, cpu_limit):
        self.stages = list()
        self.processes = set()
        self.curr_processes = dict()
        self.function_locations = dict()
        self.process_limit = process_limit
        self.mem_limit = mem_limit
        self.cpu_limit = cpu_limit
        self.curr_stage_idx = -1
        self.next_stage()

    def __iter__(self):
        self.stage = 0
        return self

    def __next__(self):
        if self.stage >= len(self.stages):
            raise StopIteration
        val = self.stages[self.stage]
        self.stage += 1
        return val

    def add_function(self, *, func_instance: int = 0, process: int = 0, req: Reservation = None):
        print("Try add function", func_instance, "to process", process)
        self.processes.add(process)
        if process not in self.curr_processes:
            if len(self.curr_processes) == self.process_limit:
                raise Exception("Adding new process " + str(process) + ", but process limit already reached")
            self.curr_processes[process] = Reservation()

        old_res = self.curr_processes[process]
        new_res = req + old_res
        if new_res.cpu > self.cpu_limit:
            raise Exception(
                f"CPU over-subscription for process {process} in stage {len(self.stages)} ({new_res.cpu} > {self.cpu_limit})")

        if new_res.mem > self.mem_limit:
            raise Exception(
                f"RAM over-subscription for process {process} in stage {len(self.stages)} ({new_res.mem} > {self.mem_limit})")
        self.curr_processes[process] = new_res
        allocation = self.FunctionAllocation(
            proc=process,
            id=func_instance,
            cpu=req.cpu,
            mem=req.mem
        )
        self.stages[self.curr_stage_idx].add(allocation)
        self.function_locations[func_instance] = process

    def next_stage(self):
        self.stages.append(set())
        self.curr_stage_idx += 1
        self.curr_processes.clear()

    def get_dependencies(self, workflow: Workflow):
        deps = dict()
        for func_list in workflow.functions.values():
            for func in func_list:
                deps[func] = dict()

        for sender, msgs in workflow.communications.items():
            sender_location = self.function_locations[sender]
            for msg in msgs:
                deps[sender][msg.dst] = self.function_locations[msg.dst]
                deps[msg.dst][sender] = sender_location

        return deps
