import logging

import os
from pathlib import Path

from confz import ConfZ, ConfZFileSource
from pydantic import AnyUrl, BaseModel

CONFIG_DIR = Path(os.getenv("RESOURCES_ROOT", "./resources"))


class AppConfig(ConfZ):
    function_store: AnyUrl
    function_dir: str
    funcwork_dir: str
    executor_id: int
    process_nameservice: str
    recv_timeout: int = 1800

    CONFIG_SOURCES = ConfZFileSource(file=CONFIG_DIR / "config.yml")


class LogConfig(BaseModel):
    """Logging configuration to be set for the server"""

    LOGGER_NAME: str = "praas-runtime"
    LOG_FORMAT: str = "%(levelprefix)s | %(asctime)s | %(message)s"
    LOG_LEVEL: str = "DEBUG"

    # Logging config
    version = 1
    disable_existing_loggers = False
    formatters = {
        "default": {
            "()": "uvicorn.logging.DefaultFormatter",
            "fmt": LOG_FORMAT,
            "datefmt": "%Y-%m-%d %H:%M:%S",
        },
    }
    handlers = {
        "default": {
            "formatter": "default",
            "class": "logging.StreamHandler",
            "stream": "ext://sys.stderr",
        },
    }
    loggers = {
        "praas-runtime": {"handlers": ["default"], "level": LOG_LEVEL},
    }


def get_logger():
    return logging.getLogger("praas-runtime")
