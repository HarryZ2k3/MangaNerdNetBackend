const output = document.getElementById("output");
const commandLog = document.getElementById("command-log");
const baseURLInput = document.getElementById("base-url");
const tokenPreview = document.getElementById("token-preview");
const tokenInput = document.getElementById("token-input");

const state = {
  baseURL: localStorage.getItem("mhBaseURL") || window.location.origin,
  token: localStorage.getItem("mhToken") || "",
  chatSocket: null,
  syncSocket: null,
};

function setOutput(title, payload) {
  const content = typeof payload === "string" ? payload : JSON.stringify(payload, null, 2);
  output.textContent = `${title}\n${content}`;
}

function appendLog(element, message) {
  const stamp = new Date().toISOString();
  element.textContent += `[${stamp}] ${message}\n`;
  element.scrollTop = element.scrollHeight;
}

function logCommand(command) {
  if (!commandLog) return;
  appendLog(commandLog, command);
}

function shellQuote(value) {
  return `'${String(value).replace(/'/g, "'\\''")}'`;
}

function buildCurlCommand(url, options, headers) {
  const method = (options.method || "GET").toUpperCase();
  const segments = ["curl"];

  if (method !== "GET") {
    segments.push(`-X ${method}`);
  }

  Object.entries(headers).forEach(([key, value]) => {
    if (key === "Content-Type" && !options.body) return;
    segments.push(`-H ${shellQuote(`${key}: ${value}`)}`);
  });

  if (options.body) {
    segments.push(`--data ${shellQuote(options.body)}`);
  }

  segments.push(shellQuote(url));
  return segments.join(" ");
}

function setToken(token) {
  state.token = token.trim();
  localStorage.setItem("mhToken", state.token);
  tokenPreview.textContent = state.token ? `${state.token.slice(0, 18)}…` : "(none)";
  tokenInput.value = state.token;
}

function setBaseURL(url) {
  state.baseURL = url.replace(/\/$/, "");
  localStorage.setItem("mhBaseURL", state.baseURL);
  baseURLInput.value = state.baseURL;
}

async function apiFetch(path, options = {}) {
  const url = `${state.baseURL}${path}`;
  const headers = {
    "Content-Type": "application/json",
    ...(options.headers || {}),
  };
  if (state.token) {
    headers.Authorization = `Bearer ${state.token}`;
  }

  logCommand(buildCurlCommand(url, options, headers));
  const response = await fetch(url, { ...options, headers });
  const text = await response.text();
  let payload = text;
  try {
    payload = text ? JSON.parse(text) : {};
  } catch (error) {
    payload = text;
  }

  if (!response.ok) {
    throw new Error(typeof payload === "string" ? payload : JSON.stringify(payload));
  }

  return payload;
}

function wsURL(path, params = {}) {
  const url = new URL(state.baseURL);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  url.pathname = path;
  url.search = new URLSearchParams(params).toString();
  return url.toString();
}

function bindForm(formId, handler) {
  const form = document.getElementById(formId);
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const data = new FormData(form);
    try {
      const result = await handler(Object.fromEntries(data.entries()));
      setOutput(`${formId} response`, result);
    } catch (error) {
      setOutput(`${formId} error`, error.message);
    }
  });
}

setBaseURL(state.baseURL);
setToken(state.token);

baseURLInput.value = state.baseURL;

document.getElementById("base-url-save").addEventListener("click", () => {
  setBaseURL(baseURLInput.value || window.location.origin);
  setOutput("Base URL", { baseURL: state.baseURL });
});

document.getElementById("health-check").addEventListener("click", async () => {
  try {
    const result = await apiFetch("/health");
    setOutput("Health", result);
  } catch (error) {
    setOutput("Health error", error.message);
  }
});

document.getElementById("ready-check").addEventListener("click", async () => {
  try {
    const result = await apiFetch("/ready");
    setOutput("Ready", result);
  } catch (error) {
    setOutput("Ready error", error.message);
  }
});

document.getElementById("token-save").addEventListener("click", () => {
  setToken(tokenInput.value);
  setOutput("Token stored", { token: state.token });
});

document.getElementById("token-clear").addEventListener("click", () => {
  setToken("");
  setOutput("Token cleared", "");
});

bindForm("register-form", async (data) => {
  const result = await apiFetch("/auth/register", {
    method: "POST",
    body: JSON.stringify(data),
  });
  if (result.token) {
    setToken(result.token);
  }
  return result;
});

bindForm("login-form", async (data) => {
  const result = await apiFetch("/auth/login", {
    method: "POST",
    body: JSON.stringify(data),
  });
  if (result.token) {
    setToken(result.token);
  }
  return result;
});

bindForm("change-password-form", async (data) => {
  return apiFetch("/auth/change-password", {
    method: "POST",
    body: JSON.stringify(data),
  });
});

document.getElementById("logout-button").addEventListener("click", async () => {
  try {
    const result = await apiFetch("/auth/logout", { method: "POST" });
    setOutput("Logout", result);
  } catch (error) {
    setOutput("Logout error", error.message);
  }
});

document.getElementById("me-button").addEventListener("click", async () => {
  try {
    const result = await apiFetch("/users/me");
    setOutput("/users/me", result);
  } catch (error) {
    setOutput("/users/me error", error.message);
  }
});

bindForm("manga-list-form", async (data) => {
  const params = new URLSearchParams();
  if (data.q) params.set("q", data.q);
  if (data.status) params.set("status", data.status);
  if (data.genres) params.set("genres", data.genres);
  if (data.limit) params.set("limit", data.limit);
  if (data.offset) params.set("offset", data.offset);
  return apiFetch(`/manga?${params.toString()}`);
});

bindForm("manga-get-form", async (data) => {
  return apiFetch(`/manga/${data.id}`);
});

bindForm("library-upsert-form", async (data) => {
  const payload = {
    manga_id: data.manga_id,
    current_chapter: data.current_chapter ? Number(data.current_chapter) : 0,
    status: data.status,
  };
  return apiFetch("/users/library", {
    method: "POST",
    body: JSON.stringify(payload),
  });
});

bindForm("library-list-form", async (data) => {
  const params = new URLSearchParams();
  if (data.status) params.set("status", data.status);
  if (data.limit) params.set("limit", data.limit);
  if (data.offset) params.set("offset", data.offset);
  return apiFetch(`/users/library?${params.toString()}`);
});

bindForm("library-get-form", async (data) => {
  return apiFetch(`/users/library/${data.manga_id}`);
});

bindForm("library-delete-form", async (data) => {
  return apiFetch(`/users/library/${data.manga_id}`, { method: "DELETE" });
});

bindForm("progress-add-form", async (data) => {
  const payload = {
    manga_id: data.manga_id,
    chapter: Number(data.chapter),
  };
  if (data.volume) {
    payload.volume = Number(data.volume);
  }
  return apiFetch("/users/progress", {
    method: "POST",
    body: JSON.stringify(payload),
  });
});

bindForm("progress-list-form", async (data) => {
  const params = new URLSearchParams();
  params.set("manga_id", data.manga_id);
  if (data.limit) params.set("limit", data.limit);
  if (data.offset) params.set("offset", data.offset);
  return apiFetch(`/users/progress?${params.toString()}`);
});

bindForm("reviews-list-form", async (data) => {
  const params = new URLSearchParams();
  if (data.limit) params.set("limit", data.limit);
  if (data.offset) params.set("offset", data.offset);
  return apiFetch(`/manga/${data.manga_id}/reviews?${params.toString()}`);
});

bindForm("reviews-create-form", async (data) => {
  const payload = {
    manga_id: data.manga_id,
    rating: Number(data.rating),
    text: data.text || "",
  };
  return apiFetch("/reviews", {
    method: "POST",
    body: JSON.stringify(payload),
  });
});

bindForm("reviews-delete-form", async (data) => {
  return apiFetch(`/reviews/${data.id}`, { method: "DELETE" });
});

const chatLog = document.getElementById("chat-log");
const syncLog = document.getElementById("sync-log");

function disconnectSocket(socket, label, log) {
  if (socket) {
    socket.close();
    appendLog(log, `${label} disconnected`);
  }
}

document.getElementById("chat-connect").addEventListener("click", () => {
  disconnectSocket(state.chatSocket, "Chat", chatLog);
  const room = document.getElementById("chat-room").value || "general";
  const user = document.getElementById("chat-user").value || "anon";
  const url = wsURL("/ws/chat", { room, user });
  const socket = new WebSocket(url);
  state.chatSocket = socket;
  logCommand(`wscat -c ${shellQuote(url)}`);
  appendLog(chatLog, `Connecting to ${url}`);
  socket.addEventListener("open", () => appendLog(chatLog, "Chat connected"));
  socket.addEventListener("message", (event) => appendLog(chatLog, event.data));
  socket.addEventListener("close", () => appendLog(chatLog, "Chat closed"));
});

document.getElementById("chat-disconnect").addEventListener("click", () => {
  disconnectSocket(state.chatSocket, "Chat", chatLog);
  state.chatSocket = null;
});

document.getElementById("chat-send").addEventListener("click", () => {
  if (!state.chatSocket || state.chatSocket.readyState !== WebSocket.OPEN) {
    appendLog(chatLog, "Chat socket not connected");
    return;
  }
  const message = document.getElementById("chat-message").value.trim();
  if (!message) return;
  logCommand(`chat send ${shellQuote(message)}`);
  state.chatSocket.send(JSON.stringify({ text: message }));
});

document.getElementById("chat-history").addEventListener("click", async () => {
  try {
    const room = document.getElementById("chat-room").value || "general";
    const result = await apiFetch(`/chat/history?room=${encodeURIComponent(room)}`);
    setOutput("Chat history", result);
  } catch (error) {
    setOutput("Chat history error", error.message);
  }
});

document.getElementById("sync-connect").addEventListener("click", () => {
  disconnectSocket(state.syncSocket, "Sync", syncLog);
  const url = wsURL("/ws");
  const socket = new WebSocket(url);
  state.syncSocket = socket;
  logCommand(`wscat -c ${shellQuote(url)}`);
  appendLog(syncLog, `Connecting to ${url}`);
  socket.addEventListener("open", () => appendLog(syncLog, "Sync connected"));
  socket.addEventListener("message", (event) => appendLog(syncLog, event.data));
  socket.addEventListener("close", () => appendLog(syncLog, "Sync closed"));
});

document.getElementById("sync-disconnect").addEventListener("click", () => {
  disconnectSocket(state.syncSocket, "Sync", syncLog);
  state.syncSocket = null;
});
