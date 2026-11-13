#!/usr/bin/env python3
"""Three real engine processes plus one server, over an explicitly supplied broker.

PLC IO and the business database use test-only fixtures. The actual MQTT
connector/source/sink, JS processor, disk buffer, SDK process host and Manager
API are exercised. No live PLC is contacted. Secrets are read only from env.
"""
import concurrent.futures
import json
import http.client
import os
import pathlib
import secrets
import signal
import socket
import sqlite3
import subprocess
import threading
import time
import urllib.error
import urllib.parse
import urllib.request

ROOT = pathlib.Path(os.environ.get("PUNKIOT_TEST_ROOT", "/home/sun/punkiot-work"))
BIN = ROOT / "artifacts"
RUN = ROOT / "runs" / (time.strftime("%Y%m%dT%H%M%S") + "-" + secrets.token_hex(3))
BROKER = os.environ["TEST_MQTT_URL"]
PREFIX = "punkiot-test/" + secrets.token_hex(8)
ADMIN = "E2e!" + secrets.token_hex(12) + "Aa9"


def wait_for(check, seconds=90):
    deadline = time.monotonic() + seconds
    last = None
    while time.monotonic() < deadline:
        try:
            value = check()
            if value:
                return value
        except Exception as exc:
            last = exc
        time.sleep(0.25)
    raise RuntimeError(f"condition timed out: {last}")


class Proxy:
    def __init__(self, host, port):
        self.remote = (host, port)
        self.online = True
        self.sockets = set()
        self.lock = threading.Lock()
        self.listener = socket.socket()
        self.listener.bind(("127.0.0.1", 0))
        self.listener.listen()
        self.port = self.listener.getsockname()[1]
        threading.Thread(target=self.accept, daemon=True).start()

    def set_online(self, online):
        with self.lock:
            self.online = online
            if not online:
                for conn in list(self.sockets):
                    try:
                        conn.shutdown(socket.SHUT_RDWR)
                    except OSError:
                        pass
                    conn.close()
                self.sockets.clear()

    def accept(self):
        while True:
            try:
                local, _ = self.listener.accept()
                with self.lock:
                    online = self.online
                if not online:
                    local.close()
                    continue
                remote = socket.create_connection(self.remote, timeout=5)
                remote.settimeout(None)
                with self.lock:
                    self.sockets.update((local, remote))
                threading.Thread(target=self.pipe, args=(local, remote), daemon=True).start()
                threading.Thread(target=self.pipe, args=(remote, local), daemon=True).start()
            except OSError:
                return

    def pipe(self, source, target):
        try:
            while True:
                data = source.recv(65536)
                if not data:
                    break
                target.sendall(data)
        except OSError:
            pass
        finally:
            with self.lock:
                self.sockets.discard(source)
                self.sockets.discard(target)
            for conn in (source, target):
                try:
                    conn.shutdown(socket.SHUT_RDWR)
                except OSError:
                    pass
                conn.close()


class Engine:
    def __init__(self, name):
        self.name = name
        self.path = RUN / name
        self.path.mkdir(parents=True)
        sock = socket.socket()
        sock.bind(("127.0.0.1", 0))
        self.port = sock.getsockname()[1]
        sock.close()
        self.token = None
        self.proc = None
        self.log = None

    def start(self):
        env = dict(os.environ, IOT_RULE_ADMIN_PASSWORD=ADMIN, IOT_RULE_DATA_DIR=str(self.path), IOT_RULE_LISTEN=f"127.0.0.1:{self.port}", IOT_RULE_PLUGINS_DIR=str(BIN / "plugins"), IOT_RULE_INSTANCE_ID=self.name, IOT_RULE_DELIVERY_MODE="reliable")
        self.log = open(self.path / "process.log", "ab")
        self.proc = subprocess.Popen([str(BIN / "iot-rule-engine")], cwd=self.path, env=env, stdout=self.log, stderr=self.log, start_new_session=True)
        wait_for(lambda: self.request("GET", "/health").get("status") == "ok", 30)
        login = self.request("POST", "/auth/login", {"username": "admin", "password": ADMIN})
        self.token = login["token"]

    def request(self, method, path, data=None):
        payload = None if data is None else json.dumps(data).encode()
        headers = {"Content-Type": "application/json"}
        if self.token:
            headers["Authorization"] = "Bearer " + self.token
        req = urllib.request.Request(f"http://127.0.0.1:{self.port}/api/v1{path}", payload, headers, method=method)
        try:
            with urllib.request.urlopen(req, timeout=45) as response:
                body = response.read()
                return json.loads(body) if body else {}
        except urllib.error.HTTPError as exc:
            # Avoid printing connector credentials or the whole request.
            raise RuntimeError(f"{self.name} {method} {path}: HTTP {exc.code}: {exc.read().decode()[:1500]}") from None
        except (http.client.RemoteDisconnected, TimeoutError):
            if self.proc and self.proc.poll() is None:
                os.kill(self.proc.pid, signal.SIGQUIT)
                time.sleep(1)
            raise

    def connector(self, broker):
        self.request("POST", "/connectors", {"id": "mqtt", "name": "Test MQTT", "plugin_id": "connect-mqtt", "session_policy": "persistent", "enabled": True, "connection": {"broker": broker, "username": os.environ.get("TEST_MQTT_USERNAME", ""), "password": os.environ.get("TEST_MQTT_PASSWORD", ""), "client_id": PREFIX.replace("/", "-") + "-" + self.name, "connect_timeout_sec": 2}})

    def rule(self, rule_id, nodes):
        self.request("POST", "/rules", {"id": rule_id, "name": rule_id, "nodes": nodes})
        self.request("POST", f"/rules/{rule_id}/deploy", {})

    def stop(self, hard=False):
        if self.proc and self.proc.poll() is None:
            os.killpg(self.proc.pid, signal.SIGKILL if hard else signal.SIGTERM)
            try:
                self.proc.wait(timeout=45)
            except subprocess.TimeoutExpired:
                os.killpg(self.proc.pid, signal.SIGKILL)
                self.proc.wait()
        if self.log:
            self.log.close()


def node(node_id, plugin, kind, config, wires=()):
    return {"id": node_id, "name": node_id, "plugin": plugin, "type": kind, "config": config, "wires": list(wires)}


def mqtt_source(node_id, topic, wires):
    return node(node_id, "source-mqtt", "source", {"connector_ref": "mqtt", "process": {"subscribe": {"topics": [topic], "qos": 1}, "decode": {"format": "json"}}, "stream": {"overflow_policy": "fail_session"}}, wires)


def mqtt_sink(node_id, topic):
    return node(node_id, "sink-mqtt", "sink", {"connector_ref": "mqtt", "target": {"topic": topic, "qos": 1}})


def count(path, query, args=()):
    with sqlite3.connect(path, timeout=5) as db:
        return db.execute(query, args).fetchone()[0]


def publish(topic, data):
    subprocess.run([str(BIN / "mqttprobe"), topic], input=json.dumps(data).encode(), check=True, timeout=20)


def main():
    RUN.mkdir(parents=True)
    remote = urllib.parse.urlsplit(BROKER)
    if remote.scheme != "tcp" or not remote.hostname:
        raise RuntimeError("This harness requires an explicit tcp:// test broker URL")
    server = Engine("server")
    edges = [Engine(f"gateway-{i:02}") for i in range(1, 4)]
    proxies = [Proxy(remote.hostname, remote.port or 1883) for _ in edges]
    results = []
    try:
        server.start()
        server.connector(BROKER)
        database = str(server.path / "received.db")
        server.rule("telemetry", [mqtt_source("mqtt-in", PREFIX + "/+/telemetry", ["audit"]), node("audit", "sink-audit", "sink", {"path": database})])
        server.rule("results", [mqtt_source("results-in", PREFIX + "/+/results", ["results-db"]), node("results-db", "sink-audit", "sink", {"path": database})])
        for edge, proxy in zip(edges, proxies):
            edge.start()
            edge.connector(f"tcp://127.0.0.1:{proxy.port}")
            if edge is edges[0]:
                proxy.set_online(False)
            for index in (1, 2):
                rule_id = f"collect-{index}"
                edge.rule(rule_id, [node("device", "source-sim", "source", {"gateway": edge.name, "device": f"device-{index}", "count": 60, "interval_ms": 500}, ["transform"]), node("transform", "javascript-transform", "processor", {"code": "function main(input) { input.payload.transformed = true; return {payload: input.payload, metadata: input.metadata}; }"}, ["disk"]), node("disk", "buffer-node", "processor", {"ack_node_id": "uplink", "retry_ms": 2000}, ["uplink"]), mqtt_sink("uplink", PREFIX + f"/{edge.name}/telemetry")])
            edge.rule("commands", [mqtt_source("commands-in", PREFIX + f"/{edge.name}/commands", ["plc"]), node("plc", "process-plc-sim", "processor", {"gateway": edge.name, "path": str(edge.path / "plc.db")}, ["result"]), mqtt_sink("result", PREFIX + f"/{edge.name}/results")])
        print("Three gateways and one server deployed; gateway-01 offline", flush=True)
        wait_for(lambda: count(database, "SELECT count(*) FROM events WHERE device != ''") >= 30)
        proxies[0].set_online(False)
        before = count(database, "SELECT count(*) FROM events WHERE gateway=?", (edges[1].name,))
        time.sleep(4)
        assert count(database, "SELECT count(*) FROM events WHERE gateway=?", (edges[1].name,)) > before
        results.append("one offline gateway does not stop another gateway")
        # Sources finish, then are disabled in the saved rules before a crash/restart.
        time.sleep(35)
        first = edges[0]
        for rule_id in ("collect-1", "collect-2"):
            first.request("POST", f"/rules/{rule_id}/disable", {})
            rule = first.request("GET", f"/rules/{rule_id}")
            rule["nodes"][0]["config"]["count"] = 0
            first.request("PUT", f"/rules/{rule_id}", rule)
            first.request("POST", f"/rules/{rule_id}/deploy", {})
        first.stop(hard=True)
        first.start()
        proxies[0].set_online(True)
        print("Gateway-01 restarted; waiting for buffered telemetry replay", flush=True)
        wait_for(lambda: count(database, "SELECT count(*) FROM events WHERE device LIKE 'device-%'") == 360, 120)
        results.append("360 unique telemetry events survive offline buffering and process restart")
        command = {"command_id": "cmd-1", "gateway_id": edges[1].name, "device_code": "plc-test", "expires_at": int(time.time()*1000)+30000, "data": {"setpoint": 3}}
        publish(PREFIX + f"/{edges[1].name}/commands", command)
        wait_for(lambda: count(edges[1].path / "plc.db", "SELECT count(*) FROM commands") == 1)
        publish(PREFIX + f"/{edges[1].name}/commands", command)
        publish(PREFIX + f"/{edges[0].name}/commands", dict(command, command_id="wrong-gateway"))
        publish(PREFIX + f"/{edges[1].name}/commands", dict(command, command_id="expired", expires_at=1))
        time.sleep(2)
        assert count(edges[1].path / "plc.db", "SELECT count(*) FROM commands") == 1
        assert count(edges[0].path / "plc.db", "SELECT count(*) FROM commands") == 0
        results.append("MQTT downlink is isolated; duplicate, expired and wrong-target writes are rejected by simulator")
        for edge in edges:
            children = subprocess.check_output(["pgrep", "-P", str(edge.proc.pid), "-a"], text=True)
            assert sum("buffer-node" in line for line in children.splitlines()) == 1
            assert sum("connect-mqtt" in line for line in children.splitlines()) == 1
        results.append("two collection rules share one buffer plugin process and one MQTT connector process per engine")
        report = {"status": "passed", "topic_prefix": PREFIX, "results": results, "limits": ["PLCs simulated; no live PLC writes", "native amd64 run; ARM64 is outside this harness", "business sink is SQLite fixture; production PostgreSQL/TimescaleDB separate"], "run_directory": str(RUN)}
        (RUN / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2))
        print(json.dumps(report, ensure_ascii=False, indent=2))
    finally:
        for engine in edges + [server]:
            engine.stop()
        for proxy in proxies:
            proxy.set_online(False)
            proxy.listener.close()


if __name__ == "__main__":
    main()
