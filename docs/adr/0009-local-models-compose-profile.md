# Local Models Compose Profile

Local `llm` and `embedding` services run behind a Compose profile rather than being required for every development startup. Day-to-day development can use cloud model providers, while the final private deployment proof enables the local-model profile with mounted GGUF model files.

