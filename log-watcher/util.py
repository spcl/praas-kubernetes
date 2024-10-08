import datetime
import json
import sys
import time
from concurrent.futures import ThreadPoolExecutor
from multiprocessing import Process

import urllib3
from kubernetes import client, config
from kubernetes.watch import watch

min_gap = datetime.timedelta(seconds=1 / 5)  # The minimum time to wait between publishing two log entries
last_publish_time = None


class PodDeletedException(Exception):
    pass


class SimpleProcessPool:
    def __init__(self):
        self.processes = []

    def terminate(self):
        sys.excepthook = lambda exctype, value, traceback: print("Bye")
        for p in self.processes:
            p.terminate()

    def start_process(self, func, args):
        p = Process(target=func, args=args)
        p.start()
        self.processes.append(p)


class Signal:
    def __init__(self):
        self.cancel_triggered = False

    def is_cancelled(self):
        return self.cancel_triggered

    def cancel(self):
        self.cancel_triggered = True


def connect_to_k8s(kube_config):
    connected_to_kubernetes = False
    while not connected_to_kubernetes:
        try:
            config.load_kube_config(config_file=kube_config)
            connected_to_kubernetes = True
        except config.config_exception.ConfigException:
            time.sleep(1)
    return client.CoreV1Api()


def watch_apps(app_labels, namespace, log_handler, kube_config=None, since_seconds=None):
    executor = SimpleProcessPool()
    for label in app_labels:
        executor.start_process(watch_app, (label, namespace, log_handler, kube_config, since_seconds))
    return executor


def watch_app(app_name, namespace, entry_handler, kube_config, since_seconds):
    executor = ThreadPoolExecutor()
    sleep_time = 10
    w = watch.Watch()
    pods_seen = set()
    pod_signals = dict()
    while True:
        try:
            v1 = connect_to_k8s(kube_config)
            print("Started watching", app_name)
            for event in w.stream(v1.list_namespaced_pod, namespace, label_selector=f"app={app_name}"):
                pod = event["object"].metadata.name
                event_type = event["type"]
                if event_type == "ADDED":
                    print("Added pod:", pod)
                    if pod not in pods_seen:
                        pods_seen.add(pod)
                        s = Signal()
                        pod_signals[pod] = s
                        executor.submit(watch_pod, pod, namespace, entry_handler, s, kube_config, since_seconds)
                elif event_type == "DELETED" or (
                        event_type == "MODIFIED" and event["object"].metadata.deletion_timestamp is not None):
                    if pod in pod_signals:
                        print("Deleted pod:", pod)
                        pod_signals[pod].cancel()
                        pod_signals.pop(pod)
                else:
                    print("event:", event["type"], "pod:", pod)

        except urllib3.exceptions.ProtocolError:
            print("Stopped watching", app_name)
            time.sleep(sleep_time)
        except Exception as e:
            print("Stopped watching", app_name, type(e), e)
            time.sleep(sleep_time)


def watch_pod(pod_name, namespace, entry_handler, signal, kube_config, since_seconds=None):
    print("  Started watching", pod_name)
    while not signal.is_cancelled():
        try:
            panic_entry = watch_log(pod_name, namespace, entry_handler, signal, kube_config,
                                    since_seconds=since_seconds)
            entry_handler(panic_entry)
            signal.cancel()
        except client.exceptions.ApiException as e:
            if e.status == 404:
                signal.cancel()
            else:
                time.sleep(1)
        except PodDeletedException:
            signal.cancel()
        except urllib3.exceptions.ProtocolError:
            signal.cancel()
        except Exception as e:
            print(type(e), e)
            signal.cancel()

    print("  Stopped watching", pod_name)


def watch_log(pod_name, namespace, entry_handler, signal, kube_config, since_seconds=None):
    print("    watch_log", pod_name)
    v1 = connect_to_k8s(kube_config)
    log_resp = v1.read_namespaced_pod_log(pod_name, namespace, follow=True, _preload_content=False,
                                          since_seconds=since_seconds)

    panic_entry = ""
    for entry in log_resp:
        if signal.is_cancelled():
            return

        # Decode
        if isinstance(entry, bytes):
            if len(entry) == 0:
                # raise client.exceptions.ApiException(status=400)
                raise Exception("entry len was 0")
            entry = entry.decode("utf8")
        else:
            raise Exception(f"Unknown response type: {type(entry)}")

        # Trim new line from end
        if entry[-1] == "\n":
            entry = entry[:-1]

        if not entry.startswith("rpc error:"):
            panic_entry = handle_log_entry(entry_handler, entry, pod_name, panic_entry)
        else:
            raise PodDeletedException()

    return panic_entry


def handle_log_entry(handler, entry, pod_name, panic_entry):
    try:
        if panic_entry == "" and entry[0] == "{":
            d = json.loads(entry)
            d["instance"] = pod_name
            if "message" not in d and "msg" in d:
                d["message"] = d["msg"]
            if "timestamp" not in d and "ts" in d:
                d["timestamp"] = datetime.datetime.fromtimestamp(d["ts"]).strftime("%Y-%m-%dT%H:%M:%S")
            if "severity" not in d and "level" in d:
                d["severity"] = d["level"]
        else:
            if entry.startswith("panic:"):
                panic_entry = entry
                print(entry)
            elif panic_entry != "":
                panic_entry += "\n" + entry
                print(entry)

            d = entry
        limit = handler(d)
        if limit:
            rate_limit()
    except Exception as e:
        print("Failed to handle entry:'", entry, "' error:", e)

    return panic_entry


def rate_limit():
    global last_publish_time
    now = datetime.datetime.now()
    if last_publish_time is not None:
        gap = now - last_publish_time
        if gap < min_gap:
            sleep_time = (min_gap - gap).total_seconds()
            # print("Sleep for:", sleep_time, "seconds")
            time.sleep(sleep_time)
    last_publish_time = now


def watch_resource_usage(handler, sample_rate=1, kube_config=None):
    v1 = connect_to_k8s(kube_config)

    # First get capacity
    node_list = v1.list_node()
    # print(node_list)

    cpu_capacities = dict()
    mem_capacities = dict()
    for node in node_list.items:
        name = node.metadata.name
        cpu_capacities[name] = cpu_to_ncpu(node.status.capacity["cpu"])
        mem_capacities[name] = mem_to_kib(node.status.capacity["memory"])
    print(cpu_capacities)
    print(mem_capacities)

    api = client.CustomObjectsApi()

    while True:
        k8s_nodes = api.list_cluster_custom_object("metrics.k8s.io", "v1beta1", "nodes")

        results = []
        for stats in k8s_nodes['items']:
            node = stats["metadata"]["name"]
            cpu_cap = cpu_capacities[node]
            mem_cap = mem_capacities[node]
            cpu = cpu_to_ncpu(stats["usage"]["cpu"])
            mem = mem_to_kib(stats["usage"]["memory"])
            results.append({
                "node": node,
                "cpu": cpu,
                "cpu_percent": to_percent(cpu, cpu_cap),
                "mem": mem,
                "mem_percent": to_percent(mem, mem_cap)
            })
        handler(results)
        time.sleep(sample_rate)


def mem_to_kib(str_val):
    if str_val.endswith("Ki"):
        return int(str_val[:-2])
    elif str_val.endswith("Mi"):
        return int(str_val[:-2]) * 1024
    else:
        raise Exception("Unknown memory units: " + str_val)


def cpu_to_ncpu(str_val):
    if str_val.isnumeric():
        return int(str_val) * 1000000000
    elif str_val.endswith("n"):
        return int(str_val[:-1])
    elif str_val.endswith("u"):
        return int(str_val[:-1]) * 1000
    else:
        raise Exception("Unknown cpu unit: " + str_val)


def to_percent(val, cap):
    return f"{(val/cap)*100:.0f}"
