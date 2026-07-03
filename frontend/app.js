const state = {
  needsSetup: false,
  user: null,
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
  if (!state.user) {
    adminStatus.textContent = "Log in as an admin to verify protected access.";
    return;
  }

  try {
    const body = await api("/api/admin/status");
    adminStatus.textContent = `Protected route is ${body.status} for ${body.role}.`;
  } catch (error) {
    adminStatus.textContent = error.code === "forbidden"
      ? "Signed in, but this account is not an admin."
      : "Protected route is unavailable.";
  }
}

document.querySelector("#auth-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const message = document.querySelector("#auth-message");
  const form = event.currentTarget;
  const payload = {
    username: form.username.value,
    password: form.password.value,
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
  await loadAuth();
});

loadStatus();
loadAuth().catch((error) => {
  document.querySelector("#auth-summary").textContent = error.message;
});
