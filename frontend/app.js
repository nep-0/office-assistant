const state = {
  needsSetup: false,
  user: null,
  selectedKnowledgeBaseId: null,
  chatSessionId: null,
  chatAbortController: null,
};

async function api(path, options = {}) {
  const response = await fetch(path, {
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    const error = new Error(body.message || "Request failed");
    error.code = body.code || "request_failed";
    error.details = body.details || {};
    throw error;
  }
  return body;
}

async function loadStatus() {
  const dependencies = document.querySelector("#dependency-list");

  try {
    const readyBody = await api("/api/ready");
    dependencies.replaceChildren(
      ...Object.entries(readyBody.dependencies).map(([name, status]) => {
        const item = document.createElement("li");
        const detail = status.mode || status.url || "";
        item.textContent = `${name}: ${status.status}${detail ? ` (${detail})` : ""}`;
        return item;
      }),
    );
  } catch (error) {
    const item = document.createElement("li");
    item.textContent = "Dependency readiness is unavailable";
    dependencies.replaceChildren(item);
  }
}

async function loadAuth() {
  const summary = document.querySelector("#auth-summary");
  const setup = await api("/api/setup/status");
  state.needsSetup = setup.needs_setup;

  try {
    const me = await api("/api/auth/me");
    state.user = me.user;
  } catch (error) {
    state.user = null;
  }

  if (state.user) {
    summary.textContent = `${state.user.username} is signed in as ${state.user.role}.`;
  } else if (state.needsSetup) {
    summary.textContent = "Create the first admin account to start using this deployment.";
  } else {
    summary.textContent = "Sign in with a local account.";
  }

  renderAuth();
  await loadKnowledgeBases();
  await loadAdminStatus();
}

function renderAuth() {
  const title = document.querySelector("#auth-title");
  const form = document.querySelector("#auth-form");
  const submit = document.querySelector("#auth-submit");
  const logout = document.querySelector("#logout-button");
  const password = document.querySelector("#password");

  title.textContent = state.needsSetup ? "First-run setup" : "Account";
  submit.textContent = state.needsSetup ? "Create admin" : "Log in";
  password.autocomplete = state.needsSetup ? "new-password" : "current-password";
  form.hidden = Boolean(state.user);
  logout.hidden = !state.user;
}

async function loadAdminStatus() {
  const adminStatus = document.querySelector("#admin-status");
  const providerPanel = document.querySelector("#provider-panel");
  if (!state.user) {
    adminStatus.textContent = "Log in as an admin to verify protected access.";
    providerPanel.hidden = true;
    return;
  }

  try {
    const body = await api("/api/admin/status");
    adminStatus.textContent = `Protected route is ${body.status} for ${body.role}.`;
    providerPanel.hidden = false;
    await loadProviderSettings();
  } catch (error) {
    adminStatus.textContent = error.code === "forbidden"
      ? "Signed in, but this account is not an admin."
      : "Protected route is unavailable.";
    providerPanel.hidden = true;
  }
}

async function loadKnowledgeBases() {
  const panel = document.querySelector("#knowledge-base-panel");
  const summary = document.querySelector("#knowledge-base-summary");
  const list = document.querySelector("#knowledge-base-list");
  if (!state.user) {
    panel.hidden = true;
    return;
  }

  panel.hidden = false;
  try {
    const body = await api("/api/knowledge-bases");
    renderKnowledgeBases(body.knowledge_bases);
    const selected = body.knowledge_bases.find((kb) => kb.id === state.selectedKnowledgeBaseId);
    if (!selected) {
      state.selectedKnowledgeBaseId = body.knowledge_bases[0]?.id || null;
    }
    summary.textContent = state.selectedKnowledgeBaseId
      ? `Selected Knowledge Base #${state.selectedKnowledgeBaseId}.`
      : "No Knowledge Base selected.";
    await loadDocuments();
  } catch (error) {
    list.replaceChildren();
    summary.textContent = "Knowledge Bases are unavailable.";
  }
}

async function loadDocuments() {
  const panel = document.querySelector("#document-panel");
  const chatPanel = document.querySelector("#chat-panel");
  const list = document.querySelector("#document-list");
  const searchForm = document.querySelector("#document-search-form");
  if (!state.selectedKnowledgeBaseId) {
    panel.hidden = true;
    chatPanel.hidden = true;
    list.replaceChildren();
    return;
  }
  panel.hidden = false;
  chatPanel.hidden = false;
  try {
    const query = searchForm.elements.q.value.trim();
    const path = query
      ? `/api/knowledge-bases/${state.selectedKnowledgeBaseId}/documents/search?q=${encodeURIComponent(query)}`
      : `/api/knowledge-bases/${state.selectedKnowledgeBaseId}/documents`;
    const body = await api(path);
    renderDocuments(body.documents);
  } catch (error) {
    const item = document.createElement("p");
    item.textContent = "Documents are unavailable.";
    list.replaceChildren(item);
  }
}

function renderDocuments(documents) {
  const list = document.querySelector("#document-list");
  if (documents.length === 0) {
    const empty = document.createElement("p");
    empty.textContent = "No documents uploaded.";
    list.replaceChildren(empty);
    return;
  }

  list.replaceChildren(
    ...documents.map((doc) => {
      const item = document.createElement("section");
      item.className = "document-item";
      const canCancel = doc.status === "pending" || doc.status === "processing";
      const canPreview = doc.status === "ready";
      const canReprocess = doc.status === "ready" || doc.status === "failed" || doc.status === "cancelled";
      item.innerHTML = `
        <div>
          <strong>${escapeText(doc.display_name)}</strong>
          <p>${escapeText(doc.status)} · ${formatBytes(doc.size_bytes)} · ${escapeText(doc.content_type)}</p>
          ${doc.error_message ? `<p>${escapeText(doc.error_message)}</p>` : ""}
        </div>
        <div class="knowledge-base-actions">
          ${canPreview ? `<button type="button" data-action="preview">Preview Markdown</button>` : ""}
          ${canReprocess ? `<button type="button" data-action="reprocess">Reprocess</button>` : ""}
          ${canCancel ? `<button type="button" data-action="cancel">Cancel ingestion</button>` : ""}
          <button type="button" data-action="delete">Delete</button>
        </div>
      `;
      item.querySelectorAll("button").forEach((button) => {
        button.addEventListener("click", () => handleDocumentAction(button.dataset.action, doc));
      });
      return item;
    }),
  );
}

async function handleDocumentAction(action, doc) {
  const message = document.querySelector("#document-message");
  const preview = document.querySelector("#markdown-preview");
  try {
    if (action === "cancel") {
      await api(`/api/documents/${doc.id}/ingestion/cancel`, { method: "POST", body: "{}" });
      message.textContent = "Cancellation requested.";
      await loadDocuments();
    }
    if (action === "preview") {
      const body = await api(`/api/documents/${doc.id}/extracted-markdown`);
      preview.textContent = body.markdown;
      preview.hidden = false;
    }
    if (action === "reprocess") {
      await api(`/api/documents/${doc.id}/reprocess`, { method: "POST", body: "{}" });
      message.textContent = "Reprocessing queued.";
      await loadDocuments();
    }
    if (action === "delete") {
      if (!window.confirm(`Delete ${doc.display_name}?`)) return;
      await api(`/api/documents/${doc.id}`, { method: "DELETE" });
      preview.hidden = true;
      message.textContent = "Deleted.";
      await loadDocuments();
    }
  } catch (error) {
    message.textContent = error.message;
  }
}

function renderKnowledgeBases(knowledgeBases) {
  const list = document.querySelector("#knowledge-base-list");
  if (knowledgeBases.length === 0) {
    const empty = document.createElement("p");
    empty.textContent = "No Knowledge Bases yet.";
    list.replaceChildren(empty);
    return;
  }

  list.replaceChildren(
    ...knowledgeBases.map((kb) => {
      const item = document.createElement("section");
      item.className = "knowledge-base-item";
      item.dataset.id = kb.id;
      item.innerHTML = `
        <div>
          <strong>${escapeText(kb.name)}</strong>
          <p>${escapeText(kb.visibility)} · owner ${escapeText(kb.owner_name)}${kb.can_write ? " · writable" : ""}</p>
        </div>
        <div class="knowledge-base-actions">
          <button type="button" data-action="select">Select</button>
          ${kb.can_write ? `<button type="button" data-action="rename">Rename</button>` : ""}
          ${state.user?.role === "admin" ? `
            <button type="button" data-action="toggle-visibility">${kb.visibility === "public" ? "Make private" : "Make public"}</button>
          ` : ""}
          ${kb.can_write ? `<button type="button" data-action="delete">Delete</button>` : ""}
        </div>
      `;
      item.querySelectorAll("button").forEach((button) => {
        button.addEventListener("click", () => handleKnowledgeBaseAction(button.dataset.action, kb));
      });
      return item;
    }),
  );
}

async function handleKnowledgeBaseAction(action, kb) {
  const message = document.querySelector("#knowledge-base-message");
  try {
    if (action === "select") {
      state.selectedKnowledgeBaseId = kb.id;
      await loadKnowledgeBases();
      return;
    }
    if (action === "rename") {
      const name = window.prompt("Knowledge Base name", kb.name);
      if (!name) return;
      await api(`/api/knowledge-bases/${kb.id}`, {
        method: "PUT",
        body: JSON.stringify({ name }),
      });
    }
    if (action === "toggle-visibility") {
      await api(`/api/knowledge-bases/${kb.id}`, {
        method: "PUT",
        body: JSON.stringify({
          name: kb.name,
          visibility: kb.visibility === "public" ? "private" : "public",
        }),
      });
    }
    if (action === "delete") {
      if (!window.confirm(`Delete ${kb.name}?`)) return;
      await api(`/api/knowledge-bases/${kb.id}`, { method: "DELETE" });
      if (state.selectedKnowledgeBaseId === kb.id) {
        state.selectedKnowledgeBaseId = null;
      }
    }
    message.textContent = "";
    await loadKnowledgeBases();
  } catch (error) {
    message.textContent = error.message;
  }
}

async function loadProviderSettings() {
  const body = await api("/api/admin/provider-settings");
  renderProviderSettings(body.settings);
}

function renderProviderSettings(settings) {
  const container = document.querySelector("#provider-forms");
  container.replaceChildren(
    ...settings.map((setting) => {
      const form = document.createElement("form");
      form.className = "form provider-form";
      form.dataset.purpose = setting.purpose;
      form.innerHTML = `
        <h3>${setting.purpose === "chat" ? "Chat model" : "Embedding model"}</h3>
        <label>
          Base URL
          <input name="base_url" value="${escapeAttribute(setting.base_url)}" required>
        </label>
        <label>
          Model
          <input name="model" value="${escapeAttribute(setting.model)}" required>
        </label>
        <label>
          API key
          <input name="api_key" type="password" autocomplete="off" placeholder="${setting.api_key_set ? setting.api_key_mask : "No key set"}">
        </label>
        <label class="checkbox-row">
          <input name="clear_api_key" type="checkbox">
          Clear saved API key
        </label>
        <button type="submit">Save ${setting.purpose}</button>
        <p class="message" role="status"></p>
      `;
      form.addEventListener("submit", saveProviderSetting);
      return form;
    }),
  );
}

async function saveProviderSetting(event) {
  event.preventDefault();
  const form = event.currentTarget;
  const message = form.querySelector(".message");
  const apiKey = form.elements.api_key.value.trim();
  const payload = {
    base_url: form.elements.base_url.value,
    model: form.elements.model.value,
    clear_api_key: form.elements.clear_api_key.checked,
  };
  if (apiKey !== "") {
    payload.api_key = apiKey;
  }

  try {
    await api(`/api/admin/provider-settings/${form.dataset.purpose}`, {
      method: "PUT",
      body: JSON.stringify(payload),
    });
    message.textContent = "Saved.";
    await loadProviderSettings();
    await loadStatus();
  } catch (error) {
    message.textContent = error.message;
  }
}

function escapeAttribute(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll('"', "&quot;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

function escapeText(value) {
  const span = document.createElement("span");
  span.textContent = String(value);
  return span.innerHTML;
}

document.querySelector("#auth-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const message = document.querySelector("#auth-message");
  const form = event.currentTarget;
  const payload = {
    username: form.elements.username.value,
    password: form.elements.password.value,
  };

  try {
    const path = state.needsSetup ? "/api/setup" : "/api/auth/login";
    const body = await api(path, {
      method: "POST",
      body: JSON.stringify(payload),
    });
    state.user = body.user;
    state.needsSetup = false;
    message.textContent = "";
    form.reset();
    await loadAuth();
  } catch (error) {
    message.textContent = error.message;
  }
});

document.querySelector("#logout-button").addEventListener("click", async () => {
  await api("/api/auth/logout", { method: "POST", body: "{}" });
  state.user = null;
  state.selectedKnowledgeBaseId = null;
  await loadAuth();
});

document.querySelector("#knowledge-base-create-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const form = event.currentTarget;
  const message = document.querySelector("#knowledge-base-message");
  try {
    const body = await api("/api/knowledge-bases", {
      method: "POST",
      body: JSON.stringify({ name: form.elements.name.value }),
    });
    state.selectedKnowledgeBaseId = body.id;
    form.reset();
    message.textContent = "";
    await loadKnowledgeBases();
  } catch (error) {
    message.textContent = error.message;
  }
});

document.querySelector("#document-upload-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  await uploadSelectedDocument(false);
});

document.querySelector("#document-search-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  await loadDocuments();
});

document.querySelector("#chat-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  await sendChatMessage(event.currentTarget);
});

document.querySelector("#chat-stop-button").addEventListener("click", async () => {
  if (state.chatAbortController) {
    state.chatAbortController.abort();
  }
  if (state.chatSessionId) {
    await api(`/api/chat-sessions/${state.chatSessionId}/cancel`, { method: "POST", body: "{}" }).catch(() => ({}));
  }
  document.querySelector("#chat-stop-button").hidden = true;
});

async function uploadSelectedDocument(confirmDuplicate) {
  const form = document.querySelector("#document-upload-form");
  const message = document.querySelector("#document-message");
  const file = form.elements.file.files[0];
  if (!file || !state.selectedKnowledgeBaseId) return;

  const body = new FormData();
  body.append("file", file);
  const suffix = confirmDuplicate ? "?confirm_duplicate=true" : "";
  const response = await fetch(`/api/knowledge-bases/${state.selectedKnowledgeBaseId}/documents/upload${suffix}`, {
    method: "POST",
    body,
  });
  const payload = await response.json().catch(() => ({}));
  if (response.status === 409 && payload.code === "duplicate_document") {
    const duplicateName = payload.details?.duplicate?.display_name || "an existing document";
    if (window.confirm(`${duplicateName} has the same content. Upload anyway?`)) {
      await uploadSelectedDocument(true);
    }
    return;
  }
  if (!response.ok) {
    message.textContent = payload.message || "Upload failed.";
    return;
  }
  form.reset();
  message.textContent = "Uploaded.";
  document.querySelector("#markdown-preview").hidden = true;
  await loadDocuments();
}

async function sendChatMessage(form) {
  if (!state.selectedKnowledgeBaseId) return;
  const message = form.elements.message.value.trim();
  if (!message) return;
  const log = document.querySelector("#chat-log");
  const status = document.querySelector("#chat-message");
  const stopButton = document.querySelector("#chat-stop-button");
  const userItem = document.createElement("div");
  userItem.className = "chat-entry user";
  userItem.textContent = message;
  const assistantItem = document.createElement("div");
  assistantItem.className = "chat-entry assistant";
  log.append(userItem, assistantItem);
  form.reset();
  status.textContent = "";
  stopButton.hidden = false;
  state.chatAbortController = new AbortController();

  try {
    const response = await fetch(`/api/knowledge-bases/${state.selectedKnowledgeBaseId}/chat`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ session_id: state.chatSessionId, message }),
      signal: state.chatAbortController.signal,
    });
    if (!response.ok || !response.body) {
      status.textContent = "Chat request failed.";
      return;
    }
    await readChatStream(response.body, assistantItem, status);
  } catch (error) {
    if (error.name !== "AbortError") {
      status.textContent = error.message;
    }
  } finally {
    state.chatAbortController = null;
    stopButton.hidden = true;
  }
}

async function readChatStream(body, assistantItem, status) {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    let boundary = buffer.indexOf("\n\n");
    while (boundary >= 0) {
      const raw = buffer.slice(0, boundary);
      buffer = buffer.slice(boundary + 2);
      handleChatEvent(raw, assistantItem, status);
      boundary = buffer.indexOf("\n\n");
    }
  }
}

function handleChatEvent(raw, assistantItem, status) {
  const lines = raw.split("\n");
  const event = lines.find((line) => line.startsWith("event: "))?.slice(7).trim();
  const dataLine = lines.find((line) => line.startsWith("data: "));
  if (!event || !dataLine) return;
  const payload = JSON.parse(dataLine.slice(6));
  if (event === "start") {
    state.chatSessionId = payload.session_id;
  }
  if (event === "retrieval") {
    status.textContent = "Searching Knowledge Base...";
  }
  if (event === "delta") {
    assistantItem.textContent += payload.text || "";
  }
  if (event === "citations") {
    status.textContent = payload.citations?.length ? "Answer includes citations." : "";
  }
  if (event === "error") {
    status.textContent = payload.message || "Chat failed.";
  }
}

function formatBytes(bytes) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

loadStatus();
loadAuth().catch((error) => {
  document.querySelector("#auth-summary").textContent = error.message;
});
