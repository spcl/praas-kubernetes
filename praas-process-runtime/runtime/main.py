from fastapi import FastAPI
from logging.config import dictConfig
import logging

from . import msg, metrics, data_plane, config

app = FastAPI()
app.include_router(msg.router, prefix="/coordinate")
app.include_router(metrics.router)
app.include_router(data_plane.router)

dictConfig(config.LogConfig().dict())
logger = logging.getLogger("praas-runtime")
