# ADK Inside Backend Application

The backend owns HTTP routes, authentication, document lifecycle, background jobs, storage, provider configuration, and UI-facing APIs, while `google.golang.org/adk` is used inside that application for agent and tool orchestration. This keeps product concerns separate from the agent runtime.

