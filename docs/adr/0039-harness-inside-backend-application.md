# Harness Inside Backend Application

The backend owns HTTP routes, authentication, document lifecycle, background jobs, storage, provider configuration, and UI-facing APIs. `github.com/nep-0/harness` owns streamed OpenAI-compatible chat completions and tool dispatch inside that application.

SQLite is the canonical conversation store. It persists user messages, assistant tool calls, tool results, final assistant messages, and citation metadata. The backend reconstructs a harness transcript for each request and does not use a second framework-owned session store.

Internal tool-call and tool-result messages are retained for valid multi-turn model context but are not returned by ordinary chat-session APIs.

The full transcript remains durable in SQLite. Harness's sliding-window context middleware limits each model request to the current turn and five preceding complete turns while preserving the leading developer instruction and complete assistant tool-call/tool-result groups. This is a turn-based initial bound; a token-aware compaction policy can replace it if model context pressure requires one later.

Harness runtime metadata injects the authenticated username and current UTC date into model-facing context without persisting either as transcript messages. A backend middleware then merges all system and developer content into one leading instruction because Qwen-compatible providers accept only one system/developer message.
