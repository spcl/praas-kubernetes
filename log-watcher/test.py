import util


def log_handler(l):
    pass


apps = ["autoscaler", "praas-controller"]
util.watch_apps(apps, "knative-serving", log_handler)
print("ended")
