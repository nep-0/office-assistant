import json
import posixpath
import tempfile
import urllib.error
import urllib.request

from markitdown import MarkItDown
from markitdown._exceptions import FileConversionException


SCHEMA_VERSION = "v1"
IMAGE_EXTENSIONS = {".png", ".jpg", ".jpeg", ".tif", ".tiff", ".webp"}
SUPPORTED_EXTENSIONS = IMAGE_EXTENSIONS | {".pdf", ".docx", ".xlsx", ".pptx"}


class ExtractionError(Exception):
    def __init__(self, code, message):
        super().__init__(message)
        self.code = code
        self.message = message


def extract_upload(filename, data, ocr_url="", ocr_func=None):
    ext = posixpath.splitext(filename.lower())[1]
    if ext not in SUPPORTED_EXTENSIONS:
        raise ExtractionError("unsupported_office_input", "file type is not supported")

    if ext in IMAGE_EXTENSIONS:
        text = call_ocr(data, filename, ocr_url, ocr_func)
        markdown = text.strip()
        ocr = {"used": True, "regions": [{"anchor_id": "image-1"}]}
    else:
        markdown = convert_with_markitdown(filename, data)
        ocr = {"used": False}

    if not markdown.strip():
        raise ExtractionError("empty_extraction", "document extraction produced no text")

    return {
        "schema_version": SCHEMA_VERSION,
        "markdown": markdown.strip(),
        "metadata": {
            "filename": filename,
            "document_type": ext.lstrip("."),
            "converter": "markitdown" if ext not in IMAGE_EXTENSIONS else "ocr",
        },
        "warnings": [],
        "ocr": ocr,
        "source_anchors": source_anchors(filename, ext),
    }


def convert_with_markitdown(filename, data):
    ext = posixpath.splitext(filename.lower())[1]
    with tempfile.NamedTemporaryFile(suffix=ext) as temp:
        temp.write(data)
        temp.flush()
        try:
            result = MarkItDown(enable_plugins=False).convert(temp.name)
        except FileConversionException as error:
            raise ExtractionError("conversion_failed", str(error)) from error
    return (getattr(result, "markdown", "") or getattr(result, "text_content", "") or "").strip()


def source_anchors(filename, ext):
    if ext == ".pdf":
        return [{"id": "page-1", "kind": "page", "label": "Page 1"}]
    if ext == ".pptx":
        return [{"id": "slide-deck", "kind": "slide", "label": "Slides"}]
    if ext == ".xlsx":
        return [{"id": "workbook", "kind": "sheet", "label": "Workbook"}]
    if ext == ".docx":
        return [{"id": "docx-body", "kind": "section", "label": "Document body"}]
    return [{"id": "image-1", "kind": "image", "label": filename}]


def call_ocr(data, filename, ocr_url, ocr_func):
    if ocr_func is not None:
        return ocr_func(data, filename)
    if not ocr_url:
        raise ExtractionError("ocr_unavailable", "OCR is required but OCR service is not configured")
    request = urllib.request.Request(
        ocr_url.rstrip("/") + "/ocr",
        data=data,
        headers={"Content-Type": "application/octet-stream", "X-Filename": ocr_header_filename(filename)},
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            payload = json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as error:
        body = error.read().decode("utf-8", errors="replace")
        raise ExtractionError("ocr_failed", ocr_error_message(body, error.reason)) from error
    except urllib.error.URLError as error:
        raise ExtractionError("ocr_unavailable", str(error.reason)) from error
    text = payload.get("text", "")
    if not text.strip():
        raise ExtractionError("ocr_empty", "OCR produced no text")
    return text


def ocr_header_filename(filename):
    fallback = "upload" + (posixpath.splitext(filename.lower())[1] or ".png")
    cleaned = posixpath.basename((filename or fallback).strip()) or fallback
    try:
        cleaned.encode("latin-1")
    except UnicodeEncodeError:
        return fallback
    return cleaned


def ocr_error_message(body, fallback):
    try:
        payload = json.loads(body)
    except json.JSONDecodeError:
        return body.strip() or str(fallback)
    return payload.get("message") or payload.get("code") or str(fallback)
