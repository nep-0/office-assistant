# Mounted GGUF Model Files

GGUF chat and embedding model files are mounted from a host model directory at runtime rather than committed to the repository, downloaded implicitly, or baked into container images. The local model containers stay model-agnostic, with expected file paths or model names supplied by configuration.

