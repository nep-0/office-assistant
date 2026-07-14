# Conversation Evidence And Debug Trace Retention

The system permanently stores the canonical conversation transcript: user messages, assistant tool calls, constrained retrieval tool results, final answers, citation evidence, and metadata needed for valid multi-turn model context and later review. Ordinary chat APIs expose only user messages, final assistant messages, errors, and citations. Detailed prompts, retrieval scores, provider events, and other verbose execution traces are stored only when debug mode is enabled, with bounded retention.
