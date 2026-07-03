# Conservative Local Concurrency

The local deployment uses small configurable worker pools with low defaults, such as one ingestion worker and one active local chat generation at a time. Cloud provider mode may allow higher chat concurrency, but default limits protect ordinary CPU deployments from embedding, OCR, and generation workloads competing unboundedly.

