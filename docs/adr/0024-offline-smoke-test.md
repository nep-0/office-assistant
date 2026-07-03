# Offline Smoke Test

The final private deployment proof is a documented offline smoke test using the local-model Compose profile with cloud provider settings disabled. The test uploads a supported document, waits for ingestion, asks a knowledge-base question, receives a cited answer, and records CPU, memory, and latency while internet access is disconnected or blocked.

