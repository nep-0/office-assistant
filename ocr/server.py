from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
import signal
import sys
from datetime import datetime, timezone


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path in ("/health", "/ready"):
            self.send_json(200, {
                "status": "ready",
                "service": "ocr",
                "mode": "fake",
                "checked_at": datetime.now(timezone.utc).isoformat(),
            })
            return

        self.send_json(404, {"code": "not_found", "message": "route not found"})

    def do_POST(self):
        if self.path == "/ocr":
            length = int(self.headers.get("Content-Length", "0"))
            if length:
                self.rfile.read(length)
            filename = self.headers.get("X-Filename", "image")
            self.send_json(200, {
                "text": f"OCR fallback text extracted from {filename}.",
                "confidence": 1.0,
            })
            return

        self.send_json(404, {"code": "not_found", "message": "route not found"})

    def send_json(self, status, body):
        payload = json.dumps(body).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, fmt, *args):
        print("%s - %s" % (self.address_string(), fmt % args))


if __name__ == "__main__":
    port = int(os.environ.get("OCR_PORT", "8082"))
    server = ThreadingHTTPServer(("", port), Handler)
    signal.signal(signal.SIGTERM, lambda _signum, _frame: sys.exit(0))
    print(f"ocr service listening on :{port}")
    server.serve_forever()
