from typing import Union

import time
import logging
import os
import sys
import traceback

from fastapi import FastAPI, HTTPException, status, Header

from . import praas, knative
from .config import AppConfig
from .models import Workflow

# setup loggers
logging_conf_path = os.path.join(os.getenv("RESOURCES_ROOT", "../resources"), "logging.conf")
logging.config.fileConfig(logging_conf_path, disable_existing_loggers=False)
logger = logging.getLogger(__name__)

app = FastAPI()


@app.get("/")
async def root():
    return {"message": "Hello World"}


@app.post("/job")
async def execute_job(scheduler: str, backend: str, workflow: Workflow, clear_cache: str = Header(default="False")):
    clear_cache = clear_cache == "True"
    try:
        start = time.perf_counter()
        config = AppConfig()
        if backend == "praas":
            results = await praas.execute(scheduler, workflow, config, clear_cache)
        elif backend == "knative":
            results = await knative.execute(workflow, config)
        else:
            raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=f"Unknown backend: {backend}")
        duration = time.perf_counter() - start
        duration *= 1000
        print(f"Job execution time: {duration}")
        return {"job": workflow.name, "status": "Done", "duration": duration, "results": results}
    except Exception as e:
        traceback.print_exc(file=sys.stderr)
        sys.stderr.flush()
        raise HTTPException(status_code=status.HTTP_500_INTERNAL_SERVER_ERROR, detail=repr(e))
