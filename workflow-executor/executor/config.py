import os
from pathlib import Path

from confz import ConfZ, ConfZFileSource
from pydantic import AnyUrl

from executor.models import mem_to_bytes

CONFIG_DIR = Path(os.getenv("RESOURCES_ROOT", "../resources"))


class AppConfig(ConfZ):
    praas_url: AnyUrl
    process_limit: int
    cpu_limit: int
    mem_limit: str
    knative_url_template: str
    redis_host: str
    redis_port: int

    CONFIG_SOURCES = ConfZFileSource(file=CONFIG_DIR / "config.yml")

    def mem_in_bytes(self):
        return mem_to_bytes(self.mem_limit)
