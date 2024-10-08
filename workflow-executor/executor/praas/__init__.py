from typing import Dict

import time

from executor.config import AppConfig
from executor.models import Workflow

from .client import DataPlaneConnectionPool
from .scheduling import algorithms
from .scheduling.types import Schedule


__connections: Dict[str, DataPlaneConnectionPool] = dict()


async def execute(scheduler: str, workflow: Workflow, config: AppConfig, clear_cache: bool):
    if scheduler not in algorithms.job_schedulers:
        raise Exception(f"Unknown scheduler: {scheduler}")

    algo_func = algorithms.job_schedulers[scheduler]
    schedule = Schedule(process_limit=config.process_limit, cpu_limit=config.cpu_limit, mem_limit=config.mem_in_bytes())
    algo_func(workflow, schedule)
    return await execute_schedule(schedule, workflow, config, clear_cache)


async def execute_schedule(schedule: Schedule, workflow: Workflow, config, clear_cache):
    if workflow.name not in __connections:
        __connections[workflow.name] = DataPlaneConnectionPool(config)
    connection_pool = __connections[workflow.name]
    if clear_cache:
        del __connections[workflow.name]

    # Create a pool of processes that we can execute on
    async with connection_pool.schedule(schedule.processes, clear_cache) as pool:
        dep_map = schedule.get_dependencies(workflow)
        func_names = workflow.get_functions()
        submit_start = time.perf_counter()
        for stage in schedule:
            for allocation in stage:
                func_id = allocation.id
                name = func_names[func_id]

                await pool.submit(
                    process=allocation.proc,
                    fid=func_id,
                    function=name,
                    args=workflow.inputs.get(func_id, []),
                    deps=dep_map.get(func_id, dict()),
                    mem=allocation.mem,
                    cpu=allocation.cpu
                )
        submit_duration = time.perf_counter() - submit_start
        print(f"Submit time: {submit_duration * 1000}")
        res_start = time.perf_counter()
        results = await pool.get_outputs(workflow.outputs)
        res_duration = time.perf_counter() - res_start
        print(f"Execution time: {res_duration * 1000}")
        return results
