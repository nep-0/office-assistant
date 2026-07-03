# OpenAI-Compatible Model API

The backend will call chat and embedding models through one OpenAI-compatible HTTP API shape, with provider base URLs and model names selected by configuration. This lets development and testing use cloud models while the final private deployment points the same backend code path at local `llm` and `embedding` services, avoiding a separate cloud integration architecture.

