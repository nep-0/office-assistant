from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from datetime import datetime, timezone


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path in ("/health", "/ready"):
            self.send_json(200, {
                "status": "ready",
                "service": "document",
                "mode": "fake",
                "checked_at": datetime.now(timezone.utc).isoformat(),
            })
            return

        self.send_json(404, {"code": "not_found", "message": "route not found"})

    def do_POST(self):
        if self.path == "/extract":
            self.send_json(200, {
                "schema_version": "v0.fake",
                "markdown": "# Fake extraction\n\nThis is deterministic placeholder content.",
                "metadata": {"service": "document", "mode": "fake"},
                "warnings": [],
                "ocr": {"used": False},
                "source_anchors": [
                    {"id": "fake-page-1", "kind": "page", "label": "Page 1"}
                ],
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
    port = int(os.environ.get("DOCUMENT_PORT", "8081"))
    server = ThreadingHTTPServer(("", port), Handler)
    print(f"document service listening on :{port}")
    server.serve_forever()
