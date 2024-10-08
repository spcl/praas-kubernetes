import os
from pathlib import Path
from string import Template

from confz import ConfZ, ConfZFileSource

CONFIG_DIR = Path(os.getenv("RESOURCES_ROOT", "../resources"))


class AppConfig(ConfZ):
    storage_path: str
    sdk_wheel: str
    template: str

    CONFIG_SOURCES = ConfZFileSource(file=CONFIG_DIR / "config.yml")

    def sdk_path(self):
        return os.path.join(CONFIG_DIR, self.sdk_wheel)

    def get_template(self) -> Template:
        templ_path = os.path.join(CONFIG_DIR, self.template)
        with open(templ_path, "r") as templ_file:
            templ_str = templ_file.read()
        return Template(templ_str)
