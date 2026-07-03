# Single-Machine Compose Deployment

The target deployment is a single-machine Compose stack with local volumes, SQLite, `chromem-go`, and mounted GGUF model files on one host. Users may access the frontend over a LAN if configured, but distributed storage, remote workers, and multi-node scaling are out of scope.

