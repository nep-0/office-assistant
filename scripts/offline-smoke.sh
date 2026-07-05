#!/usr/bin/env bash
set -euo pipefail

base_url="${BASE_URL:-http://localhost:${FRONTEND_PORT:-8080}}"
artifact_dir="${ARTIFACT_DIR:-artifacts/offline-smoke}"
username="${SMOKE_USERNAME:-smoke-admin}"
password="${SMOKE_PASSWORD:-password123}"
question="${SMOKE_QUESTION:-What does the office travel policy require for international bookings?}"
container_engine="${CONTAINER_ENGINE:-podman}"

mkdir -p "$artifact_dir"
cookie_jar="$(mktemp)"
fixture_dir="$(mktemp -d)"
trap 'rm -f "$cookie_jar"; rm -rf "$fixture_dir"' EXIT

fixture="$fixture_dir/office-policy.docx"

python3 - "$fixture" <<'PY'
import sys
import zipfile
from pathlib import Path
from xml.sax.saxutils import escape

target = Path(sys.argv[1])
paragraphs = [
    "Office Assistant Offline Smoke Fixture",
    "The office travel policy requires manager approval before any international booking.",
    "Receipts must be submitted within five business days after travel.",
]
document_xml = "".join(f"<w:p><w:r><w:t>{escape(text)}</w:t></w:r></w:p>" for text in paragraphs)
with zipfile.ZipFile(target, "w", zipfile.ZIP_DEFLATED) as docx:
    docx.writestr("[Content_Types].xml", """<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>""")
    docx.writestr("_rels/.rels", """<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>""")
    docx.writestr("word/document.xml", f"""<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>{document_xml}</w:body></w:document>""")
PY

request_json() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  if [[ -n "$body" ]]; then
    curl --fail --silent --show-error --cookie "$cookie_jar" --cookie-jar "$cookie_jar" \
      -H "Content-Type: application/json" -X "$method" --data "$body" "$base_url$path"
  else
    curl --fail --silent --show-error --cookie "$cookie_jar" --cookie-jar "$cookie_jar" \
      -X "$method" "$base_url$path"
  fi
}

json_get() {
  python3 - "$1" "$2" <<'PY'
import json
import sys
value = json.loads(sys.argv[1])
for part in sys.argv[2].split("."):
    value = value[int(part)] if isinstance(value, list) else value[part]
print(value)
PY
}

started_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
ready_before="$(request_json GET /api/ready)"

setup_status="$(request_json GET /api/setup/status)"
if [[ "$(json_get "$setup_status" needs_setup)" == "True" ]]; then
  request_json POST /api/setup "{\"username\":\"$username\",\"password\":\"$password\"}" >/dev/null
else
  request_json POST /api/auth/login "{\"username\":\"$username\",\"password\":\"$password\"}" >/dev/null
fi

kb_json="$(request_json POST /api/knowledge-bases "{\"name\":\"Offline smoke $(date -u +%Y%m%d%H%M%S)\"}")"
kb_id="$(json_get "$kb_json" id)"

upload_started_ms="$(python3 - <<'PY'
import time
print(int(time.time() * 1000))
PY
)"
upload_json="$(curl --fail --silent --show-error --cookie "$cookie_jar" --cookie-jar "$cookie_jar" \
  -F "file=@$fixture;type=application/vnd.openxmlformats-officedocument.wordprocessingml.document" \
  "$base_url/api/knowledge-bases/$kb_id/documents/upload")"
doc_id="$(json_get "$upload_json" id)"

doc_status=""
for _ in $(seq 1 "${INGESTION_POLL_ATTEMPTS:-90}"); do
  docs_json="$(request_json GET "/api/knowledge-bases/$kb_id/documents")"
  doc_status="$(python3 - "$docs_json" "$doc_id" <<'PY'
import json
import sys
body = json.loads(sys.argv[1])
doc_id = int(sys.argv[2])
for doc in body["documents"]:
    if doc["id"] == doc_id:
        print(doc["status"])
        break
PY
)"
  [[ "$doc_status" == "ready" ]] && break
  [[ "$doc_status" == "failed" || "$doc_status" == "cancelled" ]] && break
  sleep 2
done
upload_finished_ms="$(python3 - <<'PY'
import time
print(int(time.time() * 1000))
PY
)"
if [[ "$doc_status" != "ready" ]]; then
  echo "Document did not become ready; final status: $doc_status" >&2
  exit 1
fi

chat_started_ms="$(python3 - <<'PY'
import time
print(int(time.time() * 1000))
PY
)"
curl --fail --silent --show-error --cookie "$cookie_jar" --cookie-jar "$cookie_jar" \
  -H "Content-Type: application/json" \
  --data "{\"message\":\"$question\"}" \
  "$base_url/api/knowledge-bases/$kb_id/chat" > "$artifact_dir/chat.sse"
chat_finished_ms="$(python3 - <<'PY'
import time
print(int(time.time() * 1000))
PY
)"

if command -v "$container_engine" >/dev/null 2>&1; then
  "$container_engine" stats --no-stream > "$artifact_dir/container-stats.txt" 2>/dev/null || true
fi

python3 - "$artifact_dir/chat.sse" "$artifact_dir/report.json" "$artifact_dir/container-stats.txt" "$ready_before" "$started_at" "$base_url" "$kb_id" "$doc_id" "$upload_started_ms" "$upload_finished_ms" "$chat_started_ms" "$chat_finished_ms" <<'PY'
import json
import sys
from pathlib import Path

sse_path, report_path, stats_path, ready_raw, started_at, base_url, kb_id, doc_id, upload_start, upload_finish, chat_start, chat_finish = sys.argv[1:]
events = []
current = {}
for line in Path(sse_path).read_text().splitlines():
    if line.startswith("event: "):
        current["event"] = line.removeprefix("event: ")
    elif line.startswith("data: "):
        current["data"] = json.loads(line.removeprefix("data: "))
    elif not line.strip() and current:
        events.append(current)
        current = {}
if current:
    events.append(current)

citations = []
errors = []
for event in events:
    if event.get("event") == "citations":
        citations = event.get("data", {}).get("citations", [])
    if event.get("event") == "error":
        errors.append(event.get("data", {}))

report = {
    "started_at": started_at,
    "base_url": base_url,
    "readiness": json.loads(ready_raw),
    "knowledge_base_id": int(kb_id),
    "document_id": int(doc_id),
    "latency_ms": {
        "ingestion": int(upload_finish) - int(upload_start),
        "chat": int(chat_finish) - int(chat_start),
    },
    "resource_metrics": {
        "container_stats_file": stats_path if Path(stats_path).exists() else "",
        "note": "The stats file contains host-specific CPU and memory readings captured with podman or docker when available."
    },
    "chat_events": [event.get("event") for event in events],
    "citation_count": len(citations),
    "errors": errors,
}
Path(report_path).write_text(json.dumps(report, indent=2) + "\n")
if errors:
    raise SystemExit(f"chat returned errors: {errors}")
if not citations:
    raise SystemExit("chat completed without citations")
PY

echo "Offline smoke passed. Report: $artifact_dir/report.json"
