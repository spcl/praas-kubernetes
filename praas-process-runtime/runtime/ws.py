import abc
import json
from typing import Protocol

import websockets


class WebsocketBase(Protocol):
    @abc.abstractmethod
    async def send_json(self, obj):
        pass

    @abc.abstractmethod
    async def send_bytes(self, msg):
        pass

    @abc.abstractmethod
    async def send_text(self, msg: str):
        pass

    @abc.abstractmethod
    async def receive_json(self) -> dict:
        pass

    @abc.abstractmethod
    async def receive_bytes(self) -> bytes:
        pass

    @abc.abstractmethod
    async def receive_text(self) -> str:
        pass

    @abc.abstractmethod
    async def close(self):
        pass


class WebsocketChannel(WebsocketBase):
    conn: websockets.WebSocketClientProtocol

    def __init__(self, connection: websockets.WebSocketClientProtocol):
        self.conn = connection

    async def send_json(self, obj):
        await self.conn.send(json.dumps(obj))

    async def send_bytes(self, msg):
        await self.conn.send(msg)

    async def send_text(self, msg: str):
        await self.conn.send(msg)

    async def receive_json(self) -> dict:
        str_msg = await self.conn.recv()
        return json.loads(str_msg)

    async def receive_bytes(self) -> bytes:
        return await self.conn.recv()

    async def receive_text(self) -> str:
        return await self.conn.recv()

    async def close(self):
        await self.conn.close()


class NullWebsocket(WebsocketBase):
    async def send_json(self, obj):
        pass

    async def send_bytes(self, msg):
        pass

    async def send_text(self, msg: str):
        pass

    async def receive_json(self) -> dict:
        raise Exception("Null websocket cannot receive_json")

    async def receive_bytes(self) -> bytes:
        raise Exception("Null websocket cannot receive_bytes")

    async def receive_text(self) -> str:
        raise Exception("Null websocket cannot receive_text")

    async def close(self):
        pass
