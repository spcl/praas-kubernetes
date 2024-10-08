import logging
import time

from fastapi import APIRouter

from . import msg as praas_messaging, config

router = APIRouter()


class EndpointFilter(logging.Filter):
    def filter(self, record: logging.LogRecord) -> bool:
        return record.getMessage().find("/custom-metrics") == -1


# Filter out /endpoint
logging.getLogger("uvicorn.access").addFilter(EndpointFilter())


class MetricsState:
    can_accept: bool

    __last_time: float
    __funcs_ended: int
    __funcs_started: int

    def __init__(self):
        self.can_accept = True
        self.__funcs_started = 0
        self.__last_time = time.time()
        self.__logger = config.get_logger()
        self.__running_funcs = dict()

    def start_func(self, fid: int):
        self.__funcs_started += 1
        self.__running_funcs[fid] = True

    def end_func(self, fid: int):
        del self.__running_funcs[fid]

    def get_metric(self):
        now = time.time()
        interval = now - self.__last_time
        self.__last_time = now

        metric = self.__funcs_started + len(self.__running_funcs)
        self.__funcs_started = 0
        return metric, interval

    def restart(self):
        self.__funcs_started = 1
        self.__running_funcs.clear()
        self.__last_time = time.time()


metrics_state = MetricsState()


@router.get("/custom-metrics")
async def handle_metrics():
    value, interval = metrics_state.get_metric()
    # ret_val = {"metric_value": value, "interval": interval}
    ret_val = {"metric_value": value, "interval": 1}

    return ret_val


@router.get("/restart")
async def handle_restart():
    metrics_state.restart()
    return {"success": True}


@router.get("/swap")
async def handle_swap_out():
    metrics_state.can_accept = False
    await praas_messaging.notify_end_proc()
    return "OK"
