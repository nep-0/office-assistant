# Conversation Evidence And Debug Trace Retention

The system permanently stores user-visible conversations, final answers, citation evidence, and metadata needed to review document-grounded answers later. Detailed prompts, raw tool traces, and provider internals are stored only when debug mode is enabled, with bounded retention so development and evaluation remain inspectable without making verbose agent traces part of normal product data.

