"""Private TCP pass-through. Config contains addresses only, never credentials."""
import asyncio
import ipaddress
import json
import os
import socket
import sys

PRIVATE = tuple(ipaddress.ip_network(value) for value in ("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"))


def private_address(value):
    address = ipaddress.ip_address(value)
    return any(address in network for network in PRIVATE)


async def addresses(config):
    entries = await asyncio.wait_for(asyncio.get_running_loop().getaddrinfo(
        config["target_host"], config["target_port"], family=socket.AF_INET,
        type=socket.SOCK_STREAM), 10)
    if not entries or any(not private_address(entry[4][0]) for entry in entries):
        raise ValueError("target must resolve exclusively to RFC1918 private addresses")
    return list(dict.fromkeys(entry[4][0] for entry in entries))


async def pump(reader, writer):
    while True:
        data = await reader.read(65536)
        if not data:
            return
        writer.write(data)
        await writer.drain()


async def serve(config):
    allowed = tuple(ipaddress.ip_network(value) for value in config["allowed_sources"])
    active = 0

    async def forward(reader, writer):
        nonlocal active
        peer = writer.get_extra_info("peername")
        if not peer or not any(ipaddress.ip_address(peer[0]) in network for network in allowed) or active >= 256:
            writer.close()
            await writer.wait_closed()
            return
        active += 1
        upstream = None
        pumps = []
        try:
            # Resolve on every new connection so an RDS address change needs no restart.
            for address in await addresses(config):
                try:
                    remote, upstream = await asyncio.wait_for(asyncio.open_connection(address, config["target_port"]), 10)
                    break
                except (OSError, asyncio.TimeoutError):
                    continue
            if upstream is None:
                raise OSError("private target unreachable")
            pumps = [asyncio.create_task(pump(reader, upstream)), asyncio.create_task(pump(remote, writer))]
            await asyncio.wait(pumps, return_when=asyncio.FIRST_COMPLETED)
        except (OSError, ValueError, asyncio.TimeoutError) as error:
            print(type(error).__name__ + ": private target connection failed", flush=True)
        finally:
            for task in pumps:
                task.cancel()
            await asyncio.gather(*pumps, return_exceptions=True)
            for stream in (writer, upstream):
                if stream is not None:
                    stream.close()
                    try:
                        await stream.wait_closed()
                    except OSError:
                        pass
            active -= 1

    server = await asyncio.start_server(forward, config["listen_ip"], config["listen_port"], backlog=64)
    notify = os.environ.get("NOTIFY_SOCKET")
    if notify:
        with socket.socket(socket.AF_UNIX, socket.SOCK_DGRAM) as notifier:
            notifier.connect("\0" + notify[1:] if notify.startswith("@") else notify)
            notifier.sendall(b"READY=1")
    async with server:
        await server.serve_forever()


async def preflight(config):
    for address in await addresses(config):
        try:
            reader, writer = await asyncio.wait_for(asyncio.open_connection(address, config["target_port"]), 10)
            # A MySQL greeting begins with a 3-byte packet length and protocol version 10.
            greeting = await asyncio.wait_for(reader.readexactly(5), 5)
            writer.close()
            await writer.wait_closed()
            if greeting[4] != 10:
                raise ValueError("target did not send a MySQL protocol greeting")
            return
        except (OSError, asyncio.TimeoutError, asyncio.IncompleteReadError):
            continue
    raise OSError("private MySQL endpoint unreachable; check VPC routes and RDS whitelist")


async def health(config):
    # Observe the local listener without opening an unauthenticated MySQL
    # connection on every health tick.
    await addresses(config)
    address = socket.inet_aton(config["listen_ip"])[::-1].hex().upper()
    endpoint = "%s:%04X" % (address, config["listen_port"])
    with open("/proc/net/tcp", encoding="ascii") as table:
        for line in table:
            fields = line.split()
            if len(fields) > 3 and fields[1] == endpoint and fields[3] == "0A":
                return
    raise OSError("private gateway listener not found")


if __name__ == "__main__":
    with open(sys.argv[1], encoding="utf-8") as file:
        settings = json.load(file)
    mode = sys.argv[2] if len(sys.argv) > 2 else ""
    asyncio.run(preflight(settings) if mode == "--check" else health(settings) if mode == "--health" else serve(settings))
