# Isolated Document Processing Containers

Document and OCR services run with stronger container isolation where Compose or Podman makes it practical, including no backend data-volume mounts, temporary work directories, resource limits, and only necessary network access. The backend owns durable files and passes work to document processing services without exposing backend state directly.

