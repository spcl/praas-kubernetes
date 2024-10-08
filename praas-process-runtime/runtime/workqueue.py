import asyncio
from queue import Queue

from concurrent.futures import ThreadPoolExecutor
from typing import List, TypeVar, Generic, Callable, Coroutine, Awaitable

from . import config

T = TypeVar("T")
HandlerFunc = Callable[[T], Coroutine]
CallbackFunc = Callable[[T], None]
FilterFunc = Callable[[T], bool]


class WorkQueue(Generic[T]):
    wq: List[T]
    req: FilterFunc
    handler: HandlerFunc
    added_callback: CallbackFunc

    def __init__(self, handler: HandlerFunc, req: FilterFunc, new_notification: CallbackFunc):
        self.req = req
        self.wq = list()
        self.handler = handler
        self.notify_channel = Queue()
        self.added_callback = new_notification

        self.pool = ThreadPoolExecutor()
        self.pool.submit(self.__queue_checker)

    def add_task(self, task: T):
        # Add task to workqueue
        self.wq.append(task)
        self.added_callback(task)
        self.notify()

    def notify(self):
        self.notify_channel.put(True, block=False)

    def __queue_checker(self):
        while True:
            try:
                # Wait for a reason to run and then run
                try:
                    self.notify_channel.get(timeout=60)
                except:
                    pass
                self.__check_queue()
            except:
                pass

    def __check_queue(self):
        # Check if we have any work items
        if len(self.wq) == 0:
            return

        # Check if we can schedule the top work item
        next_item = self.wq[0]
        if not self.req(next_item):
            return

        # Schedule the work item
        self.wq.pop(0)
        self.pool.submit(self.__run_task, self.handler(next_item))

    def __run_task(self, task: Awaitable):
        asyncio.run(task)
        logger = config.get_logger()
        logger.debug("Loop exited")
        self.notify()
