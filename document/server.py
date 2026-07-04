from email.parser import BytesParser
from email.policy import default
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
import signal
import sys
from datetime import datetime, timezone

from extractor import ExtractionError, extract_upload


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path in ("/health", "/ready"):
            self.send_json(200, {
                "status": "ready",
                "service": "document",
                "mode": "extract",
                "checked_at": datetime.now(timezone.utc).isoformat(),
            })
            return

        self.send_json(404, {"code": "not_found", "message": "route not found"})

    def do_POST(self):
        if self.path == "/extract":
            try:
                filename, data = self.read_multipart_file()
                package = extract_upload(filename, data, os.environ.get("OCR_URL", ""))
                self.send_json(200, package)
            except ExtractionError as error:
                self.send_json(422, {"code": error.code, "message": error.message})
            except Exception as error:
                self.send_json(500, {"code": "extract_failed", "message": str(error)})
            return

        self.send_json(404, {"code": "not_found", "message": "route not found"})

    def read_multipart_file(self):
        length = int(self.headers.get("Content-Length", "0"))
        content_type = self.headers.get("Content-Type", "")
        raw = self.rfile.read(length)
        message = BytesParser(policy=default).parsebytes(
            f"Content-Type: {content_type}\nMIME-Version: 1.0\n\n".encode("utf-8") + raw
        )
        for part in message.iter_parts():
            if part.get_param("name", header="content-disposition") == "file":
                filename = part.get_filename() or "upload"
                payload = part.get_payload(decode=True) or b""
                return filename, payload
        raise ExtractionError("upload_file_required", "file field is required")

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
    signal.signal(signal.SIGTERM, lambda _signum, _frame: sys.exit(0))
    print(f"document service listening on :{port}")
    server.serve_forever()
