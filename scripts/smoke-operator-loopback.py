#!/usr/bin/env python3
"""Live localhost admin-UI walkthrough: no SDR, TX, service writes, or external network use.

This runs only against a throwaway config and a temporary loopback metrics
server, exercising a *real* sparky-config binary in a subprocess.
"""
import base64
import http.client
import http.server
import os
from pathlib import Path
import re
import socket
import subprocess
import sys
import tempfile
import threading
import time
from urllib.parse import urlencode


PASSWORD = "temporary-admin-password-for-smoke-only"
SECRET = "DONT_EXPOSE_TEST_MQTT_SECRET"
METRICS = (
    "# HELP sparky_ready Ready\n"
    "# TYPE sparky_ready gauge\n"
    "sparky_ready 1\n"
    "sparky_uptime_seconds 15.5\n"
    "sparky_pubsub_deliveries_total 8\n"
    "sparky_pubsub_dropped_deliveries_total 2\n"
)


class MetricsHandler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != "/metrics":
            self.send_error(404)
            return
        body = METRICS.encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_):
        pass


def free_port():
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def request(port, method, path, *, auth=True, data=None, origin=None):
    headers = {}
    if auth:
        token = base64.b64encode(("admin:" + PASSWORD).encode()).decode()
        headers["Authorization"] = "Basic " + token
    if data is not None:
        data = urlencode(data)
        headers["Content-Type"] = "application/x-www-form-urlencoded"
    if origin is not None:
        headers["Origin"] = origin
    conn = http.client.HTTPConnection("127.0.0.1", port, timeout=4)
    try:
        conn.request(method, path, body=data, headers=headers)
        response = conn.getresponse()
        return response.status, dict(response.getheaders()), response.read().decode("utf-8", "replace")
    finally:
        conn.close()


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def field(html, name):
    found = re.search(
        r'<input\b[^>]*\bname="' + re.escape(name) +
        r'"[^>]*\bvalue="([^"]+)"', html
    )
    require(found is not None, "missing hidden field: " + name)
    return found.group(1)


def exercise(binary):
    with tempfile.TemporaryDirectory(prefix="sparky-operator-") as tmp:
        path = Path(tmp) / "config.yml"
        path.write_text(
            "# preserve operator comment\n"
            "station-callsign: N0CALL\n"
            "openwebrx-mqtt:\n"
            "  enabled: false\n"
            "  password: " + SECRET + "\n"
            "sdr:\n"
            "  enabled: false\n"
            "transmitter:\n"
            "  enabled: false\n"
            "igate:\n"
            "  enabled: false\n"
            "enable-digipeater: false\n",
            encoding="utf-8",
        )
        path.chmod(0o600)
        # Keep the metrics listener bound before selecting the UI port so
        # our own fixture cannot claim the released UI port.
        metrics = http.server.ThreadingHTTPServer(("127.0.0.1", 0), MetricsHandler)
        thread = threading.Thread(target=metrics.serve_forever, daemon=True)
        process = None
        thread_started = False
        try:
            port = free_port()
            origin = "http://127.0.0.1:" + str(port)
            cmd = [
                str(Path(binary).resolve()), "--config", str(path),
                "--listen", "127.0.0.1:" + str(port),
                "--metrics-url", "http://127.0.0.1:" + str(metrics.server_port) + "/metrics",
            ]
            env = dict(os.environ, SPARKY_CONFIG_PASSWORD=PASSWORD)
            thread.start()
            thread_started = True
            # A file avoids deadlocking on a full stderr pipe.
            with open(Path(tmp) / "ui.stderr", "wb") as stderr:
                process = subprocess.Popen(
                    cmd, cwd=tmp, env=env,
                    stdout=subprocess.DEVNULL, stderr=stderr,
                )
            # Require a response from a live child and verify authentication below.
            until = time.monotonic() + 12
            while True:
                if process.poll() is not None:
                    raise AssertionError("UI exited during startup")
                try:
                    status, _, _ = request(port, "GET", "/", auth=False)
                    if status == 401 and process.poll() is None:
                        break
                except (ConnectionError, OSError):
                    pass
                if process.poll() is not None:
                    raise AssertionError("UI exited during startup")
                if time.monotonic() >= until:
                    raise AssertionError("UI failed to bind the loopback port")
                time.sleep(0.1)

            status, _, _ = request(port, "GET", "/", auth=False)
            require(status == 401, "overview accessible without authentication")
            status, headers, body = request(port, "GET", "/")
            require(status == 200, "authenticated overview not available")
            require("no-store" in headers.get("Cache-Control", ""),
                    "overview missing no-store")
            require("N0CALL" in body, "configured callsign not visible")
            require(SECRET not in body, "secret leaked into overview")
            for label, value in (("Subscriber deliveries", "8"), ("Dropped deliveries", "2")):
                require(re.search(
                    r'<small>\s*' + re.escape(label) +
                    r'\s*</small>\s*<div class="value">\s*' +
                    re.escape(value) + r'\s*</div>', body
                ) is not None, "incorrect metric: " + label)
            require("● READY" in body, "gateway readiness not shown")
            require("Restart service" not in body or "Restart disabled" in body,
                    "restart appears active without adapter")

            status, _, edit = request(port, "GET", "/edit")
            require(status == 200, "editor unavailable")
            require(SECRET not in edit, "secret leaked into editor")
            csrf, rev = field(edit, "csrf"), field(edit, "revision")
            submitted = {"csrf": csrf, "revision": rev, "callsign": "N0CALL-7"}
            status, _, _ = request(port, "POST", "/edit", data=submitted,
                                    origin="http://attacker.invalid")
            require(status == 403, "cross-origin save accepted")
            status, _, _ = request(port, "POST", "/edit",
                                    data=dict(submitted, csrf="invalid"),
                                    origin=origin)
            require(status == 403, "invalid CSRF token accepted")
            status, headers, _ = request(port, "POST", "/edit", data=submitted,
                                          origin=origin)
            require(status == 303, "valid editor save did not redirect")
            after = path.read_text(encoding="utf-8")
            require("N0CALL-7" in after, "saved callsign missing from YAML")
            require(SECRET in after, "editor overwrote unmodeled secret")
            require("# preserve operator comment" in after,
                    "editor dropped YAML comments")
            status, _, _ = request(port, "POST", "/edit", data=submitted,
                                    origin=origin)
            require(status == 409, "stale edit was not rejected")
            status, _, _ = request(port, "POST", "/restart",
                                    data={"csrf": csrf, "confirm": "restart"},
                                    origin=origin)
            require(status == 404, "restart unexpectedly enabled")
            status, _, body = request(port, "GET", "/")
            require(status == 200 and "N0CALL-7" in body,
                    "overview failed to display saved settings")
            print("PASS: live UI login, read-only status, edit, CSRF, origin, secrets,"
                  " conflict, and disabled restart")
        finally:
            try:
                if process is not None:
                    if process.poll() is None:
                        process.terminate()
                    try:
                        process.wait(timeout=4)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait(timeout=4)
            finally:
                if thread_started:
                    metrics.shutdown()
                    thread.join(timeout=4)
                metrics.server_close()


if __name__ == "__main__":
    binary = sys.argv[1] if len(sys.argv) == 2 else "bin/sparky-config"
    try:
        exercise(binary)
    except (AssertionError, OSError, subprocess.SubprocessError) as exc:
        print("FAIL: " + str(exc), file=sys.stderr)
        sys.exit(1)
