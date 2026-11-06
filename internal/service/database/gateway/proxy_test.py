import asyncio
import importlib.util
from pathlib import Path
import socket
import unittest
from unittest.mock import AsyncMock, mock_open, patch

spec = importlib.util.spec_from_file_location("database_gateway_proxy", Path(__file__).with_name("proxy.py"))
proxy = importlib.util.module_from_spec(spec)
spec.loader.exec_module(proxy)


class GatewayTests(unittest.IsolatedAsyncioTestCase):
    async def test_health_observes_listener_without_connecting_to_database(self):
        config = {"listen_ip": "100.80.1.1", "listen_port": 13306}
        listener = "0: 01015064:33FA 00000000:0000 0A\n"
        with patch.object(proxy, "addresses", AsyncMock(return_value=["172.16.1.2"])), patch("builtins.open", mock_open(read_data=listener)), patch.object(proxy.asyncio, "open_connection", AsyncMock()) as connect:
            await proxy.health(config)
            connect.assert_not_called()
        with patch.object(proxy, "addresses", AsyncMock(return_value=["172.16.1.2"])), patch("builtins.open", mock_open(read_data=listener.replace("0A", "01"))):
            with self.assertRaises(OSError):
                await proxy.health(config)

    async def test_dns_rejects_public_or_mixed_answers(self):
        loop = asyncio.get_running_loop()
        for addresses in (("8.8.8.8",), ("172.16.1.2", "169.254.169.254"), ("127.0.0.1",)):
            entries = [(socket.AF_INET, socket.SOCK_STREAM, 6, "", (ip, 3306)) for ip in addresses]
            with patch.object(loop, "getaddrinfo", AsyncMock(return_value=entries)):
                with self.assertRaises(ValueError):
                    await proxy.addresses({"target_host": "rds.internal", "target_port": 3306})
        entries = [(socket.AF_INET, socket.SOCK_STREAM, 6, "", ("172.16.1.2", 3306))]
        with patch.object(loop, "getaddrinfo", AsyncMock(return_value=entries)):
            self.assertEqual(await proxy.addresses({"target_host": "rds.internal", "target_port": 3306}), ["172.16.1.2"])

    async def exercise_proxy(self, allowed):
        greeting = b"\x01\x00\x00\x00\x0a"
        closed = asyncio.Event()

        async def database(reader, writer):
            try:
                writer.write(greeting)
                await writer.drain()
                while True:
                    data = await reader.read(65536)
                    if not data:
                        break
                    writer.write(data)
                    await writer.drain()
            finally:
                writer.close()
                await writer.wait_closed()
                closed.set()

        upstream = await asyncio.start_server(database, "127.0.0.1", 0)
        config = {"target_host": "rds.internal", "target_port": upstream.sockets[0].getsockname()[1], "listen_ip": "127.0.0.1", "listen_port": 0, "allowed_sources": allowed}
        original_start = asyncio.start_server
        listening = asyncio.Event()
        bound = []

        async def start(*args, **kwargs):
            server = await original_start(*args, **kwargs)
            bound.append(server)
            listening.set()
            return server

        with patch.object(proxy, "addresses", AsyncMock(return_value=["127.0.0.1"])), patch.object(proxy.asyncio, "start_server", start), patch.dict(proxy.os.environ, {}, clear=True):
            task = asyncio.create_task(proxy.serve(config))
            try:
                await asyncio.wait_for(listening.wait(), 2)
                reader, writer = await asyncio.open_connection("127.0.0.1", bound[0].sockets[0].getsockname()[1])
                if allowed == ["127.0.0.1/32"]:
                    self.assertEqual(await asyncio.wait_for(reader.readexactly(5), 2), greeting)
                    # Arbitrary binary data, including TLS-looking records, passes untouched.
                    payload = b"\x16\x03\x03\x00\x08" + bytes(range(256)) * 1024
                    writer.write(payload)
                    await writer.drain()
                    self.assertEqual(await asyncio.wait_for(reader.readexactly(len(payload)), 2), payload)
                    writer.close()
                    await writer.wait_closed()
                    await asyncio.wait_for(closed.wait(), 2)
                else:
                    self.assertEqual(await asyncio.wait_for(reader.read(), 2), b"")
                    writer.close()
                    await writer.wait_closed()
            finally:
                task.cancel()
                await asyncio.gather(task, return_exceptions=True)
                upstream.close()
                await upstream.wait_closed()

    async def test_private_forwarding_preserves_binary_and_releases_upstream(self):
        await self.exercise_proxy(["127.0.0.1/32"])

    async def test_disallowed_source_cannot_read_database(self):
        await self.exercise_proxy(["10.42.0.0/16"])


if __name__ == "__main__":
    unittest.main()
