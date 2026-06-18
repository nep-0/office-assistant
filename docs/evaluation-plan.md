# Evaluation Plan

The project evaluates whether the office assistant is usable as a CPU-only private RAG system for small and micro enterprises.

## Metric Groups

### Import and Indexing

- Parse success rate.
- Import time per document.
- Indexing time per document and per batch.
- Failed-file count and failure reason.

### Storage and Runtime

- Vector index size.
- SQLite database size.
- Peak RAM during import, indexing, and question answering.
- Idle RAM after startup.

### Retrieval Quality

- Top-k hit rate on prepared question-answer pairs.
- Comparison between vector-only retrieval and any later hybrid or reranking variant.

### Citation Quality

- Whether the cited chunk actually supports the generated answer.
- Whether document name, page number when available, and text preview are shown correctly.

### User-Facing Latency

- Retrieval time.
- LLM generation time.
- Total answer time.

## Non-Goal

The project should avoid broad claims about general answer accuracy unless the scoring rule is narrowly defined. Retrieval and citation quality are the primary measurable quality targets.

## Dataset Plan

Use a mixed dataset rather than relying only on finance QA benchmarks.

- Public subset: a small, reproducible set of public reports or QA datasets for baseline testing.
- Self-built office set: 30-100 documents covering native office documents, scanned or image-based documents, and documents with embedded images.
- Stress subset: test 100 documents first; attempt 1,000 documents only if the 100-document import and indexing time is reasonable.

Finance-oriented datasets such as ConvFinQA or TAT-DQA may be supplementary, but they should not replace office-like files because the project is an office assistant rather than a finance QA system.

## Thesis Contribution Focus

The main contribution is CPU-only private RAG optimization for office documents.

Primary experiments:

- Chunking/indexing strategy: compare chunk sizes or overlap settings using index size, retrieval hit rate, citation correctness, and latency.
- Retrieval strategy: compare vector-only retrieval against optional hybrid retrieval or optional reranking.

Model quantization should be treated as supporting infrastructure unless there is enough time to run a clean model comparison.

## Provider Reporting

Evaluation results must state which provider configuration was used:

- Fully local embedding and chat.
- Local embedding with cloud chat.
- Cloud embedding with local chat.
- Fully cloud providers.

Offline viability and interactive performance should be reported separately when local generation is slow.

## Evaluation Harness

Build a small Go or script-based CLI that runs a prepared question set against a knowledge base and exports CSV or JSON.

Each run should record:

- Provider configuration.
- Machine specifications.
- Retrieval top-k.
- Retrieved and cited chunks.
- Retrieval latency.
- Generation latency.
- Total latency.
- Manual judgment fields for retrieval hit and citation correctness.
