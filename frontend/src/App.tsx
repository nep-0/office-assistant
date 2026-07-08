import {
  Activity,
  Bot,
  Check,
  ChevronRight,
  Database,
  Download,
  Eye,
  FileText,
  Gauge,
  KeyRound,
  LogOut,
  MessageSquare,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  Send,
  Settings,
  Shield,
  Sparkles,
  Trash2,
  Upload,
  UserPlus,
  Users,
  X,
} from "lucide-react";
import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import {
  ActivityEvent,
  api,
  ApiError,
  ChatMessage,
  ChatSession,
  CitationPreview,
  DebugMode,
  DocumentRecord,
  KnowledgeBase,
  ProviderSetting,
  Role,
  RetrievalEvidence,
  streamChat,
  User,
  WorkflowMetric,
} from "./api";

type View = "knowledge" | "chat" | "admin";
type Toast = { tone: "success" | "error" | "info"; message: string };
type MessagePart = { type: "text"; content: string } | { type: "retrieval"; query: string; active: boolean };
type UIChatMessage = ChatMessage & { parts?: MessagePart[] };

const fmtDate = (value?: string) =>
  value ? new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(new Date(value)) : "Never";

const fmtBytes = (bytes: number) => {
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KB", "MB", "GB"];
  let size = bytes / 1024;
  let unit = units.shift()!;
  while (size >= 1024 && units.length) {
    size /= 1024;
    unit = units.shift()!;
  }
  return `${size.toFixed(size >= 10 ? 0 : 1)} ${unit}`;
};

function getError(error: unknown) {
  return error instanceof ApiError ? error.message : error instanceof Error ? error.message : "Something went wrong";
}

function Markdown({ content }: { content: string }) {
  return (
    <div className="markdown">
      <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
    </div>
  );
}

function AssistantMessageParts({ message, streaming }: { message: UIChatMessage; streaming: boolean }) {
  const parts = message.parts || [{ type: "text" as const, content: message.content || (streaming ? "Thinking..." : "") }];
  const hasRenderablePart = parts.some((part) => part.type === "retrieval" || part.content);

  if (!hasRenderablePart) {
    return <Markdown content={streaming ? "Thinking..." : ""} />;
  }

  return (
    <>
      {parts.map((part, index) =>
        part.type === "retrieval" ? (
          <div className={`retrieval-indicator ${part.active ? "active" : ""}`} key={`retrieval-${index}-${part.query}`}>
            <Search size={14} />
            <strong>{part.active ? "Searching" : "Searched"}</strong>
            <em>{part.query}</em>
          </div>
        ) : part.content ? (
          <Markdown content={part.content} key={`text-${index}`} />
        ) : null,
      )}
    </>
  );
}

export function App() {
  const [booting, setBooting] = useState(true);
  const [needsSetup, setNeedsSetup] = useState(false);
  const [user, setUser] = useState<User | null>(null);
  const [view, setView] = useState<View>("knowledge");
  const [toast, setToast] = useState<Toast | null>(null);

  const notify = useCallback((tone: Toast["tone"], message: string) => {
    setToast({ tone, message });
    window.setTimeout(() => setToast(null), 3600);
  }, []);

  const bootstrap = useCallback(async () => {
    setBooting(true);
    try {
      const setup = await api.setupStatus();
      setNeedsSetup(setup.needs_setup);
      if (!setup.needs_setup) {
        const me = await api.me();
        setUser(me.user);
      }
    } catch {
      setUser(null);
    } finally {
      setBooting(false);
    }
  }, []);

  useEffect(() => {
    bootstrap();
  }, [bootstrap]);

  async function logout() {
    await api.logout().catch(() => undefined);
    setUser(null);
    setView("knowledge");
  }

  if (booting) return <Splash />;

  if (!user) {
    return (
      <AuthScreen
        mode={needsSetup ? "setup" : "login"}
        onAuthenticated={(authUser) => {
          setUser(authUser);
          setNeedsSetup(false);
        }}
        notify={notify}
      />
    );
  }

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark">
            <Sparkles size={20} />
          </div>
          <div>
            <strong>Office Assistant</strong>
            <span>{user.role} workspace</span>
          </div>
        </div>
        <nav className="nav">
          <button className={view === "knowledge" ? "active" : ""} onClick={() => setView("knowledge")}>
            <Database size={18} /> Knowledge
          </button>
          <button className={view === "chat" ? "active" : ""} onClick={() => setView("chat")}>
            <MessageSquare size={18} /> Chat
          </button>
          {user.role === "admin" && (
            <button className={view === "admin" ? "active" : ""} onClick={() => setView("admin")}>
              <Shield size={18} /> Admin
            </button>
          )}
        </nav>
        <div className="profile">
          <div className="avatar">{user.username.slice(0, 2).toUpperCase()}</div>
          <div>
            <strong>{user.username}</strong>
            <span>{user.role}</span>
          </div>
          <button className="icon-button" onClick={logout} title="Log out">
            <LogOut size={17} />
          </button>
        </div>
      </aside>
      <main className={`main ${view === "admin" ? "admin-main" : ""}`}>
        {view === "knowledge" && <KnowledgeView notify={notify} />}
        {view === "chat" && <ChatView notify={notify} />}
        {view === "admin" && <AdminView notify={notify} />}
      </main>
      {toast && <div className={`toast ${toast.tone}`}>{toast.message}</div>}
    </div>
  );
}

function Splash() {
  return (
    <div className="splash">
      <div className="spinner" />
      <span>Connecting to Office Assistant</span>
    </div>
  );
}

function AuthScreen({ mode, onAuthenticated, notify }: { mode: "setup" | "login"; onAuthenticated: (user: User) => void; notify: (tone: Toast["tone"], message: string) => void }) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    try {
      const response = mode === "setup" ? await api.setup(username, password) : await api.login(username, password);
      onAuthenticated(response.user);
    } catch (error) {
      notify("error", getError(error));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="auth-screen">
      <section className="auth-visual">
        <div className="orbital">
          <div className="glass-logo">
            <Bot size={54} />
          </div>
          <div className="signal one" />
          <div className="signal two" />
          <div className="signal three" />
        </div>
        <h1>Office knowledge, ready at the desk.</h1>
        <p>Search private document collections, manage ingestion, tune model providers, and chat with cited answers.</p>
      </section>
      <form className="auth-panel" onSubmit={submit}>
        <div className="section-kicker">{mode === "setup" ? "First run" : "Welcome back"}</div>
        <h2>{mode === "setup" ? "Create the admin account" : "Sign in"}</h2>
        <label>
          Username
          <input value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="username" required />
        </label>
        <label>
          Password
          <input value={password} onChange={(event) => setPassword(event.target.value)} autoComplete={mode === "setup" ? "new-password" : "current-password"} type="password" minLength={8} required />
        </label>
        <button className="primary-button" disabled={busy}>
          <KeyRound size={18} /> {busy ? "Working..." : mode === "setup" ? "Create account" : "Sign in"}
        </button>
      </form>
    </div>
  );
}

function KnowledgeView({ notify }: { notify: (tone: Toast["tone"], message: string) => void }) {
  const [bases, setBases] = useState<KnowledgeBase[]>([]);
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [documents, setDocuments] = useState<DocumentRecord[]>([]);
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("");
  const [newName, setNewName] = useState("");
  const [editingBaseId, setEditingBaseId] = useState<number | null>(null);
  const [editingBaseName, setEditingBaseName] = useState("");
  const [loading, setLoading] = useState(true);
  const [markdown, setMarkdown] = useState<{ title: string; body: string } | null>(null);

  const selected = bases.find((base) => base.id === selectedId) || bases[0];

  const loadBases = useCallback(async () => {
    setLoading(true);
    try {
      const data = await api.knowledgeBases();
      setBases(data.knowledge_bases);
      setSelectedId((current) => current || data.knowledge_bases[0]?.id || null);
    } catch (error) {
      notify("error", getError(error));
    } finally {
      setLoading(false);
    }
  }, [notify]);

  const loadDocuments = useCallback(async () => {
    if (!selected) return;
    try {
      const params = new URLSearchParams();
      if (query) params.set("q", query);
      if (status) params.set("status", status);
      const data = query || status ? await api.searchDocuments(selected.id, params) : await api.documents(selected.id);
      setDocuments(data.documents);
    } catch (error) {
      notify("error", getError(error));
    }
  }, [notify, query, selected, status]);

  useEffect(() => {
    loadBases();
  }, [loadBases]);

  useEffect(() => {
    loadDocuments();
  }, [loadDocuments]);

  async function createBase(event: FormEvent) {
    event.preventDefault();
    if (!newName.trim()) return;
    try {
      const created = await api.createKnowledgeBase(newName.trim());
      setBases((items) => [created, ...items]);
      setSelectedId(created.id);
      setNewName("");
      notify("success", "Knowledge base created");
    } catch (error) {
      notify("error", getError(error));
    }
  }

  async function upload(file?: File) {
    if (!selected || !file) return;
    try {
      await api.uploadDocument(selected.id, file);
      notify("success", "Upload queued for ingestion");
      loadDocuments();
    } catch (error) {
      notify("error", getError(error));
    }
  }

  async function removeDocument(id: number) {
    await api.deleteDocument(id);
    notify("success", "Document deleted");
    loadDocuments();
  }

  async function renameBase(base: KnowledgeBase) {
    if (!editingBaseName.trim()) return;
    try {
      const updated = await api.updateKnowledgeBase(base.id, { name: editingBaseName.trim(), visibility: base.visibility });
      setBases((items) => items.map((item) => (item.id === base.id ? updated : item)));
      setEditingBaseId(null);
      notify("success", "Knowledge base renamed");
    } catch (error) {
      notify("error", getError(error));
    }
  }

  async function setBaseVisibility(base: KnowledgeBase, visibility: KnowledgeBase["visibility"]) {
    try {
      const updated = await api.updateKnowledgeBase(base.id, { name: base.name, visibility });
      setBases((items) => items.map((item) => (item.id === base.id ? updated : item)));
      notify("success", `Knowledge base is now ${visibility}`);
    } catch (error) {
      notify("error", getError(error));
    }
  }

  async function removeBase(base: KnowledgeBase) {
    try {
      await api.deleteKnowledgeBase(base.id);
      setBases((items) => items.filter((item) => item.id !== base.id));
      setSelectedId((current) => (current === base.id ? null : current));
      notify("success", "Knowledge base deleted");
    } catch (error) {
      notify("error", getError(error));
    }
  }

  async function showMarkdown(document: DocumentRecord) {
    try {
      const data = await api.extractedMarkdown(document.id);
      setMarkdown({ title: document.display_name, body: data.markdown });
    } catch (error) {
      notify("error", getError(error));
    }
  }

  return (
    <div className="workspace">
      <header className="topbar">
        <div>
          <div className="section-kicker">Knowledge bases</div>
          <h1>Document operations</h1>
        </div>
        <button className="ghost-button" onClick={loadBases}>
          <RefreshCw size={17} /> Refresh
        </button>
      </header>
      <div className="knowledge-grid">
        <section className="panel kb-panel">
          <form className="inline-form" onSubmit={createBase}>
            <input value={newName} onChange={(event) => setNewName(event.target.value)} placeholder="New knowledge base" />
            <button className="icon-button solid" title="Create knowledge base">
              <Plus size={18} />
            </button>
          </form>
          <div className="kb-list">
            {loading && <div className="empty">Loading collections...</div>}
            {bases.map((base) => (
              <article key={base.id} className={`kb-item ${selected?.id === base.id ? "active" : ""}`}>
                {editingBaseId === base.id ? (
                  <form className="rename-form" onSubmit={(event) => { event.preventDefault(); renameBase(base); }}>
                    <input value={editingBaseName} onChange={(event) => setEditingBaseName(event.target.value)} autoFocus />
                    <button className="icon-button solid" title="Save name">
                      <Check size={16} />
                    </button>
                    <button className="icon-button" type="button" onClick={() => setEditingBaseId(null)} title="Cancel rename">
                      <X size={16} />
                    </button>
                  </form>
                ) : (
                  <>
                    <button className="item-main" onClick={() => setSelectedId(base.id)}>
                      <span>
                        <strong>{base.name}</strong>
                        <small>{base.owner_name} · {base.visibility}</small>
                      </span>
                      <ChevronRight size={17} />
                    </button>
                    {base.can_write && (
                      <div className="item-actions">
                        <button className="icon-button" onClick={() => { setEditingBaseId(base.id); setEditingBaseName(base.name); }} title="Rename knowledge base">
                          <Pencil size={15} />
                        </button>
                        <button className="icon-button danger" onClick={() => removeBase(base)} title="Delete knowledge base">
                          <Trash2 size={15} />
                        </button>
                      </div>
                    )}
                  </>
                )}
              </article>
            ))}
            {!loading && bases.length === 0 && <div className="empty">Create a knowledge base to begin.</div>}
          </div>
        </section>
        <section className="panel documents-panel">
          {selected ? (
            <>
              <div className="panel-heading">
                <div>
                  <h2>{selected.name}</h2>
                  <p>{documents.length} documents · {selected.can_write ? "write access" : "read only"}</p>
                </div>
                <div className="panel-actions">
                  {selected.can_write && (
                    <select
                      className="visibility-select"
                      value={selected.visibility}
                      onChange={(event) => setBaseVisibility(selected, event.target.value as KnowledgeBase["visibility"])}
                      title="Knowledge base visibility"
                    >
                      <option value="private">Private</option>
                      <option value="public">Public</option>
                    </select>
                  )}
                  <label className="upload-button">
                    <Upload size={17} /> Upload
                    <input type="file" onChange={(event) => upload(event.target.files?.[0])} />
                  </label>
                </div>
              </div>
              <div className="filters">
                <label className="search-box">
                  <Search size={16} />
                  <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search documents" />
                </label>
                <select value={status} onChange={(event) => setStatus(event.target.value)}>
                  <option value="">Any status</option>
                  <option value="pending">Pending</option>
                  <option value="processing">Processing</option>
                  <option value="ready">Ready</option>
                  <option value="failed">Failed</option>
                  <option value="cancelled">Cancelled</option>
                </select>
              </div>
              <div className="document-list">
                {documents.map((document) => (
                  <article className="document-row" key={document.id}>
                    <FileText size={22} />
                    <div>
                      <strong>{document.display_name}</strong>
                      <span>{document.content_type || "file"} · {fmtBytes(document.size_bytes)} · updated {fmtDate(document.updated_at)}</span>
                      {document.error_message && <em>{document.error_message}</em>}
                    </div>
                    <span className={`status ${document.status}`}>{document.status}</span>
                    <div className="row-actions">
                      <button className="icon-button" onClick={() => showMarkdown(document)} title="View extracted markdown">
                        <Eye size={16} />
                      </button>
                      <button className="icon-button" onClick={() => api.reprocessDocument(document.id).then(loadDocuments)} title="Reprocess document">
                        <RefreshCw size={16} />
                      </button>
                      <a className="icon-button" href={`/api/documents/${document.id}/download`} title="Download original">
                        <Download size={16} />
                      </a>
                      <button className="icon-button danger" onClick={() => removeDocument(document.id)} title="Delete document">
                        <Trash2 size={16} />
                      </button>
                    </div>
                  </article>
                ))}
                {documents.length === 0 && <div className="empty">No documents match this view.</div>}
              </div>
            </>
          ) : (
            <div className="empty hero-empty">No knowledge base selected.</div>
          )}
        </section>
      </div>
      {markdown && (
        <div className="modal-backdrop" onClick={() => setMarkdown(null)}>
          <section className="modal" onClick={(event) => event.stopPropagation()}>
            <div className="modal-header">
              <div>
                <span>Document preview</span>
                <h2 title={markdown.title}>{markdown.title}</h2>
              </div>
              <button className="icon-button" onClick={() => setMarkdown(null)} title="Close">
                <X size={18} />
              </button>
            </div>
            <div className="modal-scroll">
              <Markdown content={markdown.body} />
            </div>
          </section>
        </div>
      )}
    </div>
  );
}

function ChatView({ notify }: { notify: (tone: Toast["tone"], message: string) => void }) {
  const [bases, setBases] = useState<KnowledgeBase[]>([]);
  const [baseId, setBaseId] = useState<number | "">("");
  const [sessions, setSessions] = useState<ChatSession[]>([]);
  const [sessionTitles, setSessionTitles] = useState<Record<string, string>>(() => {
    try {
      return JSON.parse(localStorage.getItem("office-assistant-session-titles") || "{}") as Record<string, string>;
    } catch {
      return {};
    }
  });
  const [editingSessionId, setEditingSessionId] = useState<string | null>(null);
  const [editingSessionTitle, setEditingSessionTitle] = useState("");
  const [sessionId, setSessionId] = useState<string | undefined>();
  const [messages, setMessages] = useState<UIChatMessage[]>([]);
  const [preview, setPreview] = useState<CitationPreview | null>(null);
  const [draft, setDraft] = useState("");
  const [streaming, setStreaming] = useState(false);

  useEffect(() => {
    api.knowledgeBases().then((data) => {
      setBases(data.knowledge_bases);
      setBaseId(data.knowledge_bases[0]?.id || "");
    }).catch((error) => notify("error", getError(error)));
  }, [notify]);

  useEffect(() => {
    if (!baseId) return;
    api.chatSessions(Number(baseId)).then((data) => setSessions(data.sessions)).catch(() => setSessions([]));
  }, [baseId]);

  async function openSession(id: string) {
    const data = await api.chatSession(id);
    setSessionId(id);
    setMessages(data.messages);
  }

  async function removeSession(id: string) {
    try {
      await api.deleteChatSession(id);
      setSessions((items) => items.filter((session) => session.id !== id));
      setSessionTitles((titles) => {
        const next = { ...titles };
        delete next[id];
        localStorage.setItem("office-assistant-session-titles", JSON.stringify(next));
        return next;
      });
      if (sessionId === id) {
        setSessionId(undefined);
        setMessages([]);
      }
      notify("success", "Session deleted");
    } catch (error) {
      notify("error", getError(error));
    }
  }

  function renameSession(id: string) {
    if (!editingSessionTitle.trim()) return;
    setSessionTitles((titles) => {
      const next = { ...titles, [id]: editingSessionTitle.trim() };
      localStorage.setItem("office-assistant-session-titles", JSON.stringify(next));
      return next;
    });
    setEditingSessionId(null);
    notify("success", "Session renamed locally");
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!baseId || !draft.trim() || streaming) return;
    const content = draft.trim();
    setDraft("");
    setStreaming(true);
    setMessages((items) => [
      ...items,
      { id: Date.now(), role: "user", content, created_at: new Date().toISOString() },
      { id: Date.now() + 1, role: "assistant", content: "", created_at: new Date().toISOString(), parts: [{ type: "text", content: "" }] },
    ]);
    try {
      await streamChat(Number(baseId), { message: content, session_id: sessionId }, (eventName, payload) => {
        const data = payload as Record<string, unknown>;
        if (eventName === "start" && typeof data.session_id === "string") setSessionId(data.session_id);
        if (eventName === "delta") {
          const delta = String(data.content || data.text || payload || "");
          setMessages((items) =>
            items.map((message, index) => {
              if (index !== items.length - 1) return message;
              const parts = [...(message.parts || [{ type: "text" as const, content: message.content }])];
              const last = parts[parts.length - 1];
              if (last?.type === "retrieval") {
                parts[parts.length - 1] = { ...last, active: false };
                parts.push({ type: "text", content: delta });
              } else if (last?.type === "text") {
                parts[parts.length - 1] = { ...last, content: last.content + delta };
              } else {
                parts.push({ type: "text", content: delta });
              }
              return { ...message, content: message.content + delta, parts };
            }),
          );
        }
        if (eventName === "retrieval" && typeof data.query === "string") {
          setMessages((items) =>
            items.map((message, index) => {
              if (index !== items.length - 1) return message;
              const parts = [...(message.parts || [{ type: "text" as const, content: message.content }])].map((part) =>
                part.type === "retrieval" ? { ...part, active: false } : part,
              );
              parts.push({ type: "retrieval", query: data.query as string, active: true });
              return { ...message, parts };
            }),
          );
        }
        if (eventName === "citations" && Array.isArray(data.citations)) {
          setMessages((items) =>
            items.map((message, index) => (index === items.length - 1 ? { ...message, citations: data.citations as RetrievalEvidence[] } : message)),
          );
        }
        if (eventName === "error") {
          notify("error", String(data.message || "Chat failed"));
        }
      });
      if (baseId) {
        const data = await api.chatSessions(Number(baseId));
        setSessions(data.sessions);
      }
    } catch (error) {
      notify("error", getError(error));
    } finally {
      setStreaming(false);
    }
  }

  return (
    <div className="workspace chat-layout">
      <aside className="panel session-panel">
        <div className="panel-heading compact">
          <h2>Sessions</h2>
          <button className="icon-button solid" onClick={() => { setSessionId(undefined); setMessages([]); }} title="New chat">
            <Plus size={17} />
          </button>
        </div>
        <select value={baseId} onChange={(event) => { setBaseId(Number(event.target.value)); setSessionId(undefined); setMessages([]); }}>
          {bases.map((base) => <option key={base.id} value={base.id}>{base.name}</option>)}
        </select>
        <div className="session-list">
          {sessions.map((session) => (
            <article key={session.id} className={`session-item ${session.id === sessionId ? "active" : ""}`}>
              {editingSessionId === session.id ? (
                <form className="rename-form" onSubmit={(event) => { event.preventDefault(); renameSession(session.id); }}>
                  <input value={editingSessionTitle} onChange={(event) => setEditingSessionTitle(event.target.value)} autoFocus />
                  <button className="icon-button solid" title="Save session name">
                    <Check size={16} />
                  </button>
                  <button className="icon-button" type="button" onClick={() => setEditingSessionId(null)} title="Cancel rename">
                    <X size={16} />
                  </button>
                </form>
              ) : (
                <>
                  <button className="item-main" onClick={() => openSession(session.id)}>
                    <MessageSquare size={17} />
                    <span>{sessionTitles[session.id] || session.title}</span>
                  </button>
                  <div className="item-actions">
                    <button className="icon-button" onClick={() => { setEditingSessionId(session.id); setEditingSessionTitle(sessionTitles[session.id] || session.title); }} title="Rename session">
                      <Pencil size={15} />
                    </button>
                    <button className="icon-button danger" onClick={() => removeSession(session.id)} title="Delete session">
                      <Trash2 size={15} />
                    </button>
                  </div>
                </>
              )}
            </article>
          ))}
        </div>
      </aside>
      <section className="panel chat-panel">
        <div className="chat-stream">
          {messages.length === 0 && (
            <div className="chat-empty">
              <Bot size={42} />
              <h1>Ask across your office knowledge.</h1>
              <p>Answers stream from the selected knowledge base and keep citations close by.</p>
            </div>
          )}
          {messages.map((message) => (
            <article key={`${message.id}-${message.created_at}`} className={`bubble ${message.role}`}>
              <span>{message.role}</span>
              {message.role === "assistant" ? (
                <AssistantMessageParts message={message} streaming={streaming} />
              ) : (
                <p>{message.content}</p>
              )}
              {message.role === "assistant" && message.citations && message.citations.length > 0 && (
                <div className="citation-tags">
                  {message.citations.map((citation, citationIndex) => (
                    <button
                      key={`${message.id}-${citation.citation_id}-${citationIndex}`}
                      onClick={() => sessionId && api.citationPreview(sessionId, citation.citation_id).then(setPreview).catch((error) => notify("error", getError(error)))}
                    >
                      <FileText size={14} />
                      {citation.document_name}
                    </button>
                  ))}
                </div>
              )}
            </article>
          ))}
        </div>
        <form className="composer" onSubmit={submit}>
          <textarea value={draft} onChange={(event) => setDraft(event.target.value)} placeholder="Ask a question about the selected knowledge base" />
          <button className="primary-button" disabled={!draft.trim() || !baseId || streaming}>
            <Send size={18} /> Send
          </button>
        </form>
      </section>
      {preview && (
        <div className="modal-backdrop" onClick={() => setPreview(null)}>
          <section className="modal citation-modal" onClick={(event) => event.stopPropagation()}>
            <div className="modal-header">
              <div>
                <span>Citation detail</span>
                <h2 title={preview.document_name}>{preview.document_name}</h2>
                <p>{preview.heading_path || preview.citation_id}</p>
              </div>
              <button className="icon-button" onClick={() => setPreview(null)} title="Close">
                <X size={18} />
              </button>
            </div>
            <div className="modal-scroll">
              <Markdown content={preview.text} />
            </div>
            <a className="primary-button" href={preview.original_download_url}>
              <FileText size={17} /> Original
            </a>
          </section>
        </div>
      )}
    </div>
  );
}

function AdminView({ notify }: { notify: (tone: Toast["tone"], message: string) => void }) {
  const [status, setStatus] = useState<string>("checking");
  const [debug, setDebug] = useState<DebugMode | null>(null);
  const [providers, setProviders] = useState<ProviderSetting[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [events, setEvents] = useState<ActivityEvent[]>([]);
  const [metrics, setMetrics] = useState<WorkflowMetric[]>([]);

  const load = useCallback(async () => {
    try {
      const [admin, debugData, providerData, usersData, activityData, metricData] = await Promise.all([
        api.adminStatus(),
        api.debug(),
        api.providerSettings(),
        api.adminUsers(),
        api.activity(),
        api.metrics(),
      ]);
      setStatus(admin.status);
      setDebug(debugData);
      setProviders(providerData.settings);
      setUsers(usersData.users);
      setEvents(activityData.events);
      setMetrics(metricData.metrics);
    } catch (error) {
      notify("error", getError(error));
    }
  }, [notify]);

  useEffect(() => {
    load();
  }, [load]);

  const avgMetric = useMemo(() => {
    if (!metrics.length) return 0;
    return Math.round(metrics.reduce((sum, metric) => sum + metric.value_ms, 0) / metrics.length);
  }, [metrics]);

  async function saveProvider(provider: ProviderSetting, apiKey?: string, clearApiKey?: boolean) {
    try {
      await api.updateProviderSetting(provider.purpose, {
        base_url: provider.base_url,
        model: provider.model,
        ...(apiKey ? { api_key: apiKey } : {}),
        ...(clearApiKey ? { clear_api_key: true } : {}),
      });
      notify("success", "Provider saved");
      load();
    } catch (error) {
      notify("error", getError(error));
    }
  }

  return (
    <div className="workspace admin-layout">
      <header className="topbar full">
        <div>
          <div className="section-kicker">Admin</div>
          <h1>System controls</h1>
        </div>
        <button className="ghost-button" onClick={load}>
          <RefreshCw size={17} /> Refresh
        </button>
      </header>
      <section className="stat-grid">
        <div className="stat">
          <Shield size={22} />
          <span>Route</span>
          <strong>{status}</strong>
        </div>
        <div className="stat">
          <Gauge size={22} />
          <span>Mean workflow</span>
          <strong>{avgMetric} ms</strong>
        </div>
        <div className="stat">
          <Activity size={22} />
          <span>Events</span>
          <strong>{events.length}</strong>
        </div>
      </section>
      <section className="panel">
        <div className="panel-heading">
          <div>
            <h2>Debug mode</h2>
            <p>{debug ? `${debug.source} · retention ${debug.retention_hours}h` : "Loading"}</p>
          </div>
          <button className={`toggle ${debug?.enabled ? "on" : ""}`} disabled={!debug || debug.environment_locked} onClick={() => debug && api.setDebug(!debug.enabled).then(setDebug)}>
            <span />
          </button>
        </div>
      </section>
      <section className="panel">
        <div className="panel-heading">
          <h2>Model providers</h2>
        </div>
        <div className="provider-grid">
          {providers.map((provider, index) => (
            <ProviderEditor
              key={provider.purpose}
              provider={provider}
              onChange={(next) => setProviders((items) => items.map((item, itemIndex) => (itemIndex === index ? next : item)))}
              onSave={saveProvider}
            />
          ))}
        </div>
      </section>
      <UserManagement users={users} setUsers={setUsers} notify={notify} />
      <section className="panel two-column">
        <div>
          <h2>Recent activity</h2>
          <div className="timeline">
            {events.slice(0, 8).map((event) => (
              <article key={event.id}>
                <Check size={15} />
                <div>
                  <strong>{event.event_type}</strong>
                  <span>{event.entity_type} {event.entity_id} · {fmtDate(event.created_at)}</span>
                </div>
              </article>
            ))}
          </div>
        </div>
        <div>
          <h2>Workflow metrics</h2>
          <div className="metric-list">
            {metrics.slice(0, 8).map((metric) => (
              <article key={metric.id}>
                <span>{metric.name}</span>
                <strong>{metric.value_ms} ms</strong>
              </article>
            ))}
          </div>
        </div>
      </section>
    </div>
  );
}

function UserManagement({
  users,
  setUsers,
  notify,
}: {
  users: User[];
  setUsers: (users: User[] | ((users: User[]) => User[])) => void;
  notify: (tone: Toast["tone"], message: string) => void;
}) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<Role>("member");
  const [editingId, setEditingId] = useState<number | null>(null);
  const [editUsername, setEditUsername] = useState("");
  const [editPassword, setEditPassword] = useState("");
  const [editRole, setEditRole] = useState<Role>("member");

  async function createUser(event: FormEvent) {
    event.preventDefault();
    try {
      const response = await api.createAdminUser({ username: username.trim(), password, role });
      setUsers((items) => [...items, response.user].sort((a, b) => a.username.localeCompare(b.username)));
      setUsername("");
      setPassword("");
      setRole("member");
      notify("success", "User created");
    } catch (error) {
      notify("error", getError(error));
    }
  }

  async function saveUser(user: User) {
    const body: { username?: string; password?: string; role?: Role } = {};
    if (editUsername.trim() && editUsername.trim() !== user.username) body.username = editUsername.trim();
    if (editPassword) body.password = editPassword;
    if (editRole !== user.role) body.role = editRole;
    try {
      const response = await api.updateAdminUser(user.id, body);
      setUsers((items) => items.map((item) => (item.id === user.id ? response.user : item)));
      setEditingId(null);
      setEditPassword("");
      notify("success", "User updated");
    } catch (error) {
      notify("error", getError(error));
    }
  }

  async function deleteUser(id: number) {
    try {
      await api.deleteAdminUser(id);
      setUsers((items) => items.filter((item) => item.id !== id));
      notify("success", "User deleted");
    } catch (error) {
      notify("error", getError(error));
    }
  }

  return (
    <section className="panel users-panel">
      <div className="panel-heading">
        <div>
          <h2>Users</h2>
          <p>{users.length} accounts</p>
        </div>
        <Users size={22} />
      </div>
      <form className="user-create-form" onSubmit={createUser}>
        <input value={username} onChange={(event) => setUsername(event.target.value)} placeholder="Username" required />
        <input value={password} onChange={(event) => setPassword(event.target.value)} placeholder="Temporary password" minLength={8} type="password" required />
        <select value={role} onChange={(event) => setRole(event.target.value as Role)}>
          <option value="member">Member</option>
          <option value="admin">Admin</option>
        </select>
        <button className="primary-button">
          <UserPlus size={17} /> Add
        </button>
      </form>
      <div className="user-list">
        {users.map((user) => (
          <article className="user-row" key={user.id}>
            {editingId === user.id ? (
              <form className="user-edit-form" onSubmit={(event) => { event.preventDefault(); saveUser(user); }}>
                <input value={editUsername} onChange={(event) => setEditUsername(event.target.value)} />
                <input value={editPassword} onChange={(event) => setEditPassword(event.target.value)} placeholder="New password optional" minLength={8} type="password" />
                <select value={editRole} onChange={(event) => setEditRole(event.target.value as Role)}>
                  <option value="member">Member</option>
                  <option value="admin">Admin</option>
                </select>
                <button className="icon-button solid" title="Save user">
                  <Check size={16} />
                </button>
                <button className="icon-button" onClick={() => setEditingId(null)} title="Cancel edit" type="button">
                  <X size={16} />
                </button>
              </form>
            ) : (
              <>
                <div className="user-identity">
                  <strong>{user.username}</strong>
                  <span>{user.role}</span>
                </div>
                <div className="row-actions">
                  <button className="icon-button" onClick={() => { setEditingId(user.id); setEditUsername(user.username); setEditRole(user.role); setEditPassword(""); }} title="Edit user">
                    <Pencil size={15} />
                  </button>
                  <button className="icon-button danger" onClick={() => deleteUser(user.id)} title="Delete user">
                    <Trash2 size={15} />
                  </button>
                </div>
              </>
            )}
          </article>
        ))}
      </div>
    </section>
  );
}

function ProviderEditor({
  provider,
  onChange,
  onSave,
}: {
  provider: ProviderSetting;
  onChange: (provider: ProviderSetting) => void;
  onSave: (provider: ProviderSetting, apiKey?: string, clearApiKey?: boolean) => void;
}) {
  const [apiKey, setApiKey] = useState("");
  const [clearApiKey, setClearApiKey] = useState(false);

  return (
    <article className="provider-card">
      <div>
        <strong>{provider.purpose}</strong>
        <span>{provider.api_key_set ? provider.api_key_mask || "API key set" : "No API key"}</span>
      </div>
      <label>
        Base URL
        <input value={provider.base_url} onChange={(event) => onChange({ ...provider, base_url: event.target.value })} />
      </label>
      <label>
        Model
        <input value={provider.model} onChange={(event) => onChange({ ...provider, model: event.target.value })} />
      </label>
      <label>
        API key
        <input
          value={apiKey}
          onChange={(event) => {
            setApiKey(event.target.value);
            if (event.target.value) setClearApiKey(false);
          }}
          placeholder={provider.api_key_set ? "Leave blank to keep current key" : "Paste provider API key"}
          type="password"
        />
      </label>
      <label className="checkbox-label">
        <input checked={clearApiKey} disabled={Boolean(apiKey)} onChange={(event) => setClearApiKey(event.target.checked)} type="checkbox" />
        Clear saved API key
      </label>
      <button className="primary-button" onClick={() => onSave(provider, apiKey.trim() || undefined, clearApiKey)}>
        <Settings size={17} /> Save
      </button>
    </article>
  );
}
