# Risk Register

## Top Risk: CPU/RAM Too Weak for Local Models

The largest project risk is that the target CPU-only machine cannot comfortably run the local chat model, embedding model, OCR service, document processor, Go backend, frontend, SQLite, and vector index together.

## Mitigation

Run a minimal local-model proof before deep UI polish:

- Start a llama.cpp-compatible chat container.
- Start a llama.cpp-compatible embedding container.
- Call both through the Go provider interfaces.
- Measure RAM usage, startup time, embedding latency, first-token latency, and total generation time.

## Fallback

Keep provider configuration flexible:

- Fully local embedding and chat for the offline target.
- Local embedding with cloud chat if generation is too slow.
- Cloud embedding with local chat if embedding throughput is the bottleneck.
- Fully cloud providers as an admin-enabled optional mode.

Evaluation results must clearly state which provider configuration was used.

## Demo Strategy If Local Generation Is Slow

The demo should still show the fully local path working on a small prompt and document, even if response time is slow. Cloud chat may be shown separately as an admin-enabled performance mode.

The report should separate offline viability from interactive performance.
