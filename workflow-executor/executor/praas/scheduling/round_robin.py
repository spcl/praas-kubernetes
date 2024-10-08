from . import util
from .types import Schedule
from executor.models import Workflow


def round_robin(workflow: Workflow, schedule: Schedule) -> None:
    """ We will allocate functions to processes in a round-robin fashion """
    requirements = util.get_requirements(workflow)
    stages = util.get_stages(workflow)

    for stage in stages:
        for instance in stage:
            allocated = False
            try_proc = 1
            while not allocated:
                try:
                    schedule.add_function(func_instance=instance, process=try_proc, req=requirements[instance])
                    allocated = True
                except:
                    if try_proc == schedule.process_limit:
                        schedule.next_stage()
                        try_proc = 1
                    else:
                        try_proc += 1
        schedule.next_stage()


def maximal_use(workflow: Workflow, schedule: Schedule) -> None:
    """ Try to run job on as many nodes as possible """
    requirements = util.get_requirements(workflow)
    stages = util.get_stages(workflow)

    curr_proc = 1
    for stage in stages:
        for instance in stage:
            allocated = False
            while not allocated:
                try:
                    schedule.add_function(func_instance=instance, process=curr_proc, req=requirements[instance])
                    allocated = True
                except:
                    if curr_proc == schedule.process_limit:
                        schedule.next_stage()
                        curr_proc = 1
                    else:
                        curr_proc += 1
            curr_proc += 1
            if curr_proc > schedule.process_limit:
                curr_proc = 1
        schedule.next_stage()
