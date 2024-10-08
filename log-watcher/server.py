import atexit
import json
from datetime import datetime

import toml

import util

from flask import Flask, render_template
from flask_sse import sse

app = Flask(__name__)
app.config.from_file("config.toml", load=toml.load)

# Server Sent Events
app.config["REDIS_URL"] = "redis://localhost"
app.register_blueprint(sse, url_prefix="/log-stream")


@app.route("/")
def index():
    return render_template("index.html")


ignore_list = ["E", "I", "redis:"]
dict_ignore = ["Reconcile succeeded"]


def log_handler(log_entry):
    with app.app_context():
        publish = True
        if isinstance(log_entry, dict):
            for ignore in dict_ignore:
                publish &= not log_entry["message"].startswith(ignore)
            # print("publish:", log_entry["message"])
            log_entry = json.dumps(log_entry)
        elif isinstance(log_entry, str):
            for x in ignore_list:
                publish &= not log_entry.startswith(x)
            if publish and log_entry.startswith("panic:"):
                panic_lines = log_entry.split("\n")
                if len(panic_lines) > 1:
                    log_dict = dict()
                    log_dict["message"] = panic_lines[0]
                    log_dict["stacktrace"] = log_entry
                    log_dict["severity"] = "fatal"
                    log_dict["timestamp"] = datetime.now().strftime("%Y-%m-%dT%H:%M:%S")
                    log_entry = json.dumps(log_dict)

        if publish:
            sse.publish(log_entry, type="logAddition")
        return publish


since_seconds = 1800
kube_cfg = None
if "KUBECONFIG" in app.config:
    kube_cfg = app.config["KUBECONFIG"]

pool = util.watch_apps(app.config["APP_LABELS"], app.config["NAMESPACE"], log_handler, kube_config=kube_cfg, since_seconds=since_seconds)
atexit.register(lambda: pool.terminate())
