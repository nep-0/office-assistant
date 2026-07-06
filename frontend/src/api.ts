export type Role = "admin" | "member";
export type Visibility = "private" | "public";
export type DocumentStatus = "pending" | "processing" | "ready" | "failed" | "cancelled";
export type ProviderPurpose = "chat" | "embedding";

export interface User {
  id: number;
  username: string;
  role: Role;
}

export interface AuthResponse {
  user: User;
}

export interface KnowledgeBase {
  id: number;
  name: string;
  visibility: Visibility;
  owner_id: number;
  owner_name: string;
  can_write: boolean;
  created_at: string;
  updated_at: string;
}

export interface DocumentRecord {
  id: number;
  knowledge_base_id: number;
  original_filename: string;
  display_name: string;
  content_type: string;
  size_bytes: number;
  status: DocumentStatus;
  error_code?: string;
  error_message?: string;
  created_at: string;
  updated_at: string;
}

export interface ChatSession {
  id: string;
  knowledge_base_id: number;
  title: string;
  created_at: string;
  updated_at: string;
}

export interface RetrievalEvidence {
  citation_id: string;
  document_id: number;
  document_name: string;
  heading_path?: string;
  source_anchor?: Record<string, unknown>;
  text: string;
}

export interface CitationPreview {
  session_id: string;
  citation_id: string;
  document_id: number;
  document_name: string;
  heading_path?: string;
  source_anchor?: Record<string, unknown>;
  text: string;
  original_download_url: string;
}

export interface ChatMessage {
  id: number;
  role: "user" | "assistant" | "tool" | "error";
  content: string;
  citations?: RetrievalEvidence[];
  created_at: string;
}

export interface ChatSessionDetail {
  session: ChatSession;
  messages: ChatMessage[];
}

export interface ProviderSetting {
  purpose: ProviderPurpose;
  base_url: string;
  model: string;
  api_key_set: boolean;
  api_key_mask?: string;
  updated_at: string;
}

export interface DebugMode {
  enabled: boolean;
  source: string;
  environment_locked: boolean;
  retention_hours: number;
}

export interface ActivityEvent {
  id: number;
  user_id: number;
  event_type: string;
  entity_type: string;
  entity_id: string;
  details?: Record<string, unknown>;
  created_at: string;
}

export interface WorkflowMetric {
  id: number;
  name: string;
  value_ms: number;
  count: number;
  details?: Record<string, unknown>;
  created_at: string;
}

export class ApiError extends Error {
  code: string;
  details?: Record<string, unknown>;

  constructor(message: string, code = "request_failed", details?: Record<string, unknown>) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.details = details;
  }
}

const jsonHeaders = { "Content-Type": "application/json" };

async function parseResponse<T>(response: Response): Promise<T> {
  const text = await response.text();
  const data = text ? JSON.parse(text) : {};
  if (!response.ok) {
    throw new ApiError(data.message || response.statusText, data.code, data.details);
  }
  return data as T;
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    credentials: "include",
    ...init,
    headers: {
      ...(init.body instanceof FormData ? {} : jsonHeaders),
      ...init.headers,
    },
  });
  return parseResponse<T>(response);
}

export const api = {
  setupStatus: () => request<{ needs_setup: boolean }>("/api/setup/status"),
  setup: (username: string, password: string) =>
    request<AuthResponse>("/api/setup", { method: "POST", body: JSON.stringify({ username, password }) }),
  login: (username: string, password: string) =>
    request<AuthResponse>("/api/auth/login", { method: "POST", body: JSON.stringify({ username, password }) }),
  logout: () => request<{ status: string }>("/api/auth/logout", { method: "POST" }),
  me: () => request<AuthResponse>("/api/auth/me"),
  health: () => request<{ status: string; service: string; started_at: string }>("/api/health"),
  ready: () => request<{ status: "ready" | "degraded"; dependencies: Record<string, { status: string; message?: string }> }>("/api/ready"),
  adminStatus: () => request<{ status: string; role: "admin" }>("/api/admin/status"),
  activity: () => request<{ events: ActivityEvent[] }>("/api/admin/activity"),
  metrics: () => request<{ metrics: WorkflowMetric[] }>("/api/admin/metrics"),
  debug: () => request<DebugMode>("/api/admin/debug"),
  setDebug: (enabled: boolean) => request<DebugMode>("/api/admin/debug", { method: "PUT", body: JSON.stringify({ enabled }) }),
  providerSettings: () => request<{ settings: ProviderSetting[] }>("/api/admin/provider-settings"),
  updateProviderSetting: (purpose: ProviderPurpose, body: { base_url: string; model: string; api_key?: string; clear_api_key?: boolean }) =>
    request<ProviderSetting>(`/api/admin/provider-settings/${purpose}`, { method: "PUT", body: JSON.stringify(body) }),
  knowledgeBases: () => request<{ knowledge_bases: KnowledgeBase[] }>("/api/knowledge-bases"),
  createKnowledgeBase: (name: string) => request<KnowledgeBase>("/api/knowledge-bases", { method: "POST", body: JSON.stringify({ name }) }),
  updateKnowledgeBase: (id: number, body: { name: string; visibility?: Visibility }) =>
    request<KnowledgeBase>(`/api/knowledge-bases/${id}`, { method: "PUT", body: JSON.stringify(body) }),
  deleteKnowledgeBase: (id: number) => request<{ status: string }>(`/api/knowledge-bases/${id}`, { method: "DELETE" }),
  documents: (knowledgeBaseId: number) => request<{ documents: DocumentRecord[] }>(`/api/knowledge-bases/${knowledgeBaseId}/documents`),
  searchDocuments: (knowledgeBaseId: number, params: URLSearchParams) =>
    request<{ documents: DocumentRecord[] }>(`/api/knowledge-bases/${knowledgeBaseId}/documents/search?${params.toString()}`),
  uploadDocument: (knowledgeBaseId: number, file: File, confirmDuplicate = false) => {
    const body = new FormData();
    body.append("file", file);
    return request<DocumentRecord>(`/api/knowledge-bases/${knowledgeBaseId}/documents/upload?confirm_duplicate=${confirmDuplicate}`, { method: "POST", body });
  },
  deleteDocument: (id: number) => request<{ status: "deleted" }>(`/api/documents/${id}`, { method: "DELETE" }),
  reprocessDocument: (id: number) => request<{ status: "pending" }>(`/api/documents/${id}/reprocess`, { method: "POST" }),
  cancelIngestion: (id: number) => request<{ status: string }>(`/api/documents/${id}/ingestion/cancel`, { method: "POST" }),
  extractedMarkdown: (id: number) => request<{ document_id: number; markdown: string }>(`/api/documents/${id}/extracted-markdown`),
  chatSessions: (knowledgeBaseId: number) => request<{ sessions: ChatSession[] }>(`/api/knowledge-bases/${knowledgeBaseId}/chat-sessions`),
  chatSession: (sessionId: string) => request<ChatSessionDetail>(`/api/chat-sessions/${sessionId}`),
  deleteChatSession: (sessionId: string) => request<{ status: "deleted" }>(`/api/chat-sessions/${sessionId}`, { method: "DELETE" }),
  cancelChat: (sessionId: string) => request<{ status: "cancel_requested" }>(`/api/chat-sessions/${sessionId}/cancel`, { method: "POST" }),
  citationPreview: (sessionId: string, citationId: string) =>
    request<CitationPreview>(`/api/chat-sessions/${sessionId}/citations/${encodeURIComponent(citationId)}/preview`),
};

export async function streamChat(
  knowledgeBaseId: number,
  body: { message: string; session_id?: string },
  onEvent: (event: string, data: unknown) => void,
): Promise<void> {
  const response = await fetch(`/api/knowledge-bases/${knowledgeBaseId}/chat`, {
    method: "POST",
    credentials: "include",
    headers: jsonHeaders,
    body: JSON.stringify(body),
  });
  if (!response.ok || !response.body) {
    await parseResponse(response);
    return;
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const chunks = buffer.split("\n\n");
    buffer = chunks.pop() || "";

    for (const chunk of chunks) {
      let event = "message";
      const dataLines: string[] = [];
      for (const line of chunk.split("\n")) {
        if (line.startsWith("event:")) event = line.slice(6).trim();
        if (line.startsWith("data:")) dataLines.push(line.slice(5).trim());
      }
      const raw = dataLines.join("\n");
      if (!raw) continue;
      try {
        onEvent(event, JSON.parse(raw));
      } catch {
        onEvent(event, raw);
      }
    }
  }
}
