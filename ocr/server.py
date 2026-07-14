from http.server import BaseHTTPRequestHandler, HTTPServer
import importlib.util
import json
import os
import posixpath
import signal
import sys
import tempfile
from datetime import datetime, timezone
from pathlib import Path


IMAGE_EXTENSIONS = {".png", ".jpg", ".jpeg", ".tif", ".tiff", ".webp"}
_OCR_ENGINE = None


class OCRError(Exception):
    def __init__(self, code, message, status=500):
        super().__init__(message)
        self.code = code
        self.message = message
        self.status = status


def clean_filename(filename):
    cleaned = posixpath.basename((filename or "image").strip())
    if cleaned in ("", ".", "/"):
        return "image.png"
    return cleaned


def extension_for(filename):
    ext = posixpath.splitext(filename.lower())[1]
    if ext in IMAGE_EXTENSIONS:
        return ext
    return ".png"


def fake_ocr(data, filename):
    if not data:
        raise OCRError("ocr_empty_input", "OCR input is empty", 400)
    return {
        "text": f"OCR fallback text extracted from {filename}.",
        "confidence": 1.0,
        "engine": "fake",
    }


def ppocr_available():
    return importlib.util.find_spec("paddleocr") is not None and importlib.util.find_spec("paddle") is not None


def ppocr_ready():
    return _OCR_ENGINE is not None


def get_ppocr():
    global _OCR_ENGINE
    if _OCR_ENGINE is None:
        try:
            from paddleocr import PaddleOCR
        except ImportError as error:
            raise OCRError("ppocr_unavailable", "PaddleOCR is not installed", 503) from error
        _OCR_ENGINE = PaddleOCR(
            text_detection_model_name=os.environ.get(
                "PADDLEOCR_DETECTION_MODEL",
                "PP-OCRv5_mobile_det",
            ),
            text_detection_model_dir=os.environ.get(
                "PADDLEOCR_DETECTION_MODEL_DIR",
                "/opt/ocr-models/official_models/PP-OCRv5_mobile_det",
            ),
            text_recognition_model_name=os.environ.get(
                "PADDLEOCR_RECOGNITION_MODEL",
                "en_PP-OCRv5_mobile_rec",
            ),
            text_recognition_model_dir=os.environ.get(
                "PADDLEOCR_RECOGNITION_MODEL_DIR",
                "/opt/ocr-models/official_models/en_PP-OCRv5_mobile_rec",
            ),
            device=os.environ.get("PADDLEOCR_DEVICE", "cpu"),
            use_doc_orientation_classify=False,
            use_doc_unwarping=False,
            use_textline_orientation=False,
        )
    return _OCR_ENGINE


def run_ppocr(data, filename):
    if not data:
        raise OCRError("ocr_empty_input", "OCR input is empty", 400)
    clean = clean_filename(filename)
    suffix = extension_for(clean)
    with tempfile.TemporaryDirectory() as workdir:
        input_path = Path(workdir) / ("input" + suffix)
        input_path.write_bytes(data)
        try:
            result = get_ppocr().predict(str(input_path))
        except OCRError:
            raise
        except Exception as error:
            raise OCRError("ppocr_failed", str(error), 502) from error

    texts = collect_values(result, "rec_texts")
    if not texts:
        texts = collect_values(result, "text")
    lines = [str(text).strip() for text in flatten(texts) if str(text).strip()]
    if not lines:
        raise OCRError("ocr_empty", "PaddleOCR produced no text", 422)

    scores = [float(score) for score in flatten(collect_values(result, "rec_scores")) if is_number(score)]
    confidence = sum(scores) / len(scores) if scores else 1.0
    return {
        "text": "\n".join(lines),
        "confidence": confidence,
        "engine": "ppocr",
    }


def collect_values(value, key, depth=0):
    if depth > 8 or value is None:
        return []
    if isinstance(value, dict):
        found = []
        if key in value:
            found.append(value[key])
        for nested in value.values():
            found.extend(collect_values(nested, key, depth + 1))
        return found
    if isinstance(value, (list, tuple)):
        found = []
        for item in value:
            found.extend(collect_values(item, key, depth + 1))
        return found
    found = []
    for attr in ("json", "res"):
        if hasattr(value, attr):
            found.extend(collect_values(getattr(value, attr), key, depth + 1))
    if hasattr(value, "to_dict"):
        try:
            found.extend(collect_values(value.to_dict(), key, depth + 1))
        except Exception:
            pass
    return found


def flatten(value):
    if isinstance(value, (list, tuple)):
        for item in value:
            yield from flatten(item)
    else:
        yield value


def is_number(value):
    try:
        float(value)
        return True
    except (TypeError, ValueError):
        return False


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/health":
            self.send_json(200, {
                "status": "alive",
                "service": "ocr",
                "checked_at": datetime.now(timezone.utc).isoformat(),
            })
            return

        if self.path == "/ready":
            mode = os.environ.get("OCR_MODE", "ppocr")
            available = ppocr_available()
            ready = mode == "fake" or ppocr_ready()
            status = "ready" if ready else "initializing" if available else "degraded"
            self.send_json(200 if ready else 503, {
                "status": status,
                "service": "ocr",
                "mode": mode,
                "ppocr_available": available,
                "ppocr_ready": ready,
                "checked_at": datetime.now(timezone.utc).isoformat(),
            })
            return

        self.send_json(404, {"code": "not_found", "message": "route not found"})

    def do_POST(self):
        if self.path == "/ocr":
            length = int(self.headers.get("Content-Length", "0"))
            data = self.rfile.read(length) if length else b""
            filename = clean_filename(self.headers.get("X-Filename", "image"))
            try:
                if os.environ.get("OCR_MODE", "ppocr") == "fake":
                    result = fake_ocr(data, filename)
                else:
                    result = run_ppocr(data, filename)
                self.send_json(200, result)
            except OCRError as error:
                self.send_json(error.status, {"code": error.code, "message": error.message})
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
    if os.environ.get("OCR_MODE", "ppocr") != "fake":
        print("initializing PaddleOCR")
        try:
            get_ppocr()
            print("PaddleOCR ready")
        except OCRError as error:
            print(f"PaddleOCR initialization failed: {error.message}", file=sys.stderr)
    server = HTTPServer(("", port), Handler)
    signal.signal(signal.SIGTERM, lambda _signum, _frame: sys.exit(0))
    print(f"ocr service listening on :{port}")
    server.serve_forever()
