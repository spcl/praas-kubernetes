import sys

import traceback

import asyncio

import aiohttp

from executor.config import AppConfig
from executor.models import Workflow


async def execute(workflow: Workflow, config: AppConfig):
    timeout = aiohttp.ClientTimeout(total=600)
    async with aiohttp.ClientSession(timeout=timeout) as session:
        tasks = dict()
        for func_name, instances in workflow.functions.items():
            func_name = func_name.replace("_", "-")
            func_url = config.knative_url_template.format(func_name)
            for fid in instances:
                args = workflow.inputs.get(fid, [])
                func_input = {
                    "id": fid,
                    "args": args,
                    "redis_host": config.redis_host,
                    "redis_port": config.redis_port
                }
                task = asyncio.create_task(session.post(func_url, json=func_input))
                tasks[fid] = task

        results = dict()
        for fid, task in tasks.items():
            try:
                print("Wait for function:", fid, file=sys.stderr, flush=True)
                response = await task
                print("Function:", fid, "finished", file=sys.stderr, flush=True)
                if not response.ok:
                    print(await response.text())
                    raise Exception(f"Function {fid} failed: {response}")

                if fid in workflow.outputs:
                    result = await response.json()
                    results[fid] = result
            except:
                traceback.print_exc(file=sys.stderr)
                sys.stderr.flush()
                raise
    return results
