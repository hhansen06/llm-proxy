const el = {
  apiBase: document.getElementById("apiBase"),
  adminToken: document.getElementById("adminToken"),
  reloadBtn: document.getElementById("reloadBtn"),
  workerForm: document.getElementById("workerForm"),
  tokenForm: document.getElementById("tokenForm"),
  workerMsg: document.getElementById("workerMsg"),
  tokenMsg: document.getElementById("tokenMsg"),
  workersTable: document.getElementById("workersTable"),
  tokensTable: document.getElementById("tokensTable"),
  usageTable: document.getElementById("usageTable"),
  logsFilterForm: document.getElementById("logsFilterForm"),
  logsTable: document.getElementById("logsTable"),
  statWorkers: document.getElementById("statWorkers"),
  statModels: document.getElementById("statModels"),
  statTokens: document.getElementById("statTokens"),
  statReq1m: document.getElementById("statReq1m"),
  statTok1m: document.getElementById("statTok1m"),
  statTok24h: document.getElementById("statTok24h"),
};

const logFilters = {
  token_id: "",
  model: "",
  limit: "100",
};

const STORAGE_KEY = "llm-proxy-admin-ui";

function saveConfig() {
  localStorage.setItem(
    STORAGE_KEY,
    JSON.stringify({ apiBase: el.apiBase.value, adminToken: el.adminToken.value })
  );
}

function loadConfig() {
  const raw = localStorage.getItem(STORAGE_KEY);
  if (!raw) return;
  try {
    const parsed = JSON.parse(raw);
    if (parsed.apiBase) el.apiBase.value = parsed.apiBase;
    if (parsed.adminToken) el.adminToken.value = parsed.adminToken;
  } catch (_) {
    // ignore invalid storage
  }
}

function baseHeaders() {
  return {
    Authorization: `Bearer ${el.adminToken.value.trim()}`,
    "Content-Type": "application/json",
  };
}

async function api(path, options = {}) {
  const res = await fetch(`${el.apiBase.value.replace(/\/$/, "")}${path}`, {
    ...options,
    headers: {
      ...baseHeaders(),
      ...(options.headers || {}),
    },
  });
  const text = await res.text();
  let body = null;
  try {
    body = text ? JSON.parse(text) : null;
  } catch (_) {
    body = { raw: text };
  }
  if (!res.ok) {
    throw new Error(body?.error || `HTTP ${res.status}`);
  }
  return body;
}

function asInputNumber(v) {
  if (v === "" || v == null) return null;
  const n = Number(v);
  return Number.isFinite(n) ? n : null;
}

function fmtTime(v) {
  if (!v) return "-";
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return "-";
  return d.toLocaleString();
}

function escapeHtml(v) {
  return String(v)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function renderWorkers(workers) {
  const rows = workers
    .map((w) => {
      const models = (w.models || []).map((m) => `<span class="tag">${escapeHtml(m)}</span>`).join(" ");
      const statusClass = w.status === "degraded" ? "warn" : "";
      return `
        <tr>
          <td>${w.id}</td>
          <td>${escapeHtml(w.name || "-")}</td>
          <td>${escapeHtml(w.base_url || "-")}</td>
          <td><span class="tag ${statusClass}">${escapeHtml(w.status || "unknown")}</span></td>
          <td>${w.capacity_hint ?? "-"}</td>
          <td>${w.last_latency_ms ?? "-"}</td>
          <td>${models || "-"}</td>
          <td>
            <div class="row-actions">
              <button data-action="refresh" data-id="${w.id}">Refresh</button>
              <button data-action="deactivate" data-id="${w.id}">Deactivate</button>
            </div>
          </td>
        </tr>
      `;
    })
    .join("");

  el.workersTable.innerHTML = `
    <table>
      <thead>
        <tr>
          <th>ID</th>
          <th>Name</th>
          <th>URL</th>
          <th>Status</th>
          <th>Cap</th>
          <th>Latency</th>
          <th>Models</th>
          <th>Aktion</th>
        </tr>
      </thead>
      <tbody>${rows || '<tr><td colspan="8">Keine Worker</td></tr>'}</tbody>
    </table>
  `;
}

function renderTokens(tokens) {
  const rows = tokens
    .map((t) => {
      return `
        <tr>
          <td>${t.id}</td>
          <td>${escapeHtml(t.label || "-")}</td>
          <td>${t.tenant_id}</td>
          <td>${t.quota_requests_per_min ?? "-"}</td>
          <td>${t.quota_tokens_per_day ?? "-"}</td>
          <td><span class="tag ${t.debug_enabled ? "warn" : ""}">${t.debug_enabled ? "on" : "off"}</span></td>
          <td><span class="tag ${t.is_revoked ? "warn" : ""}">${t.is_revoked ? "revoked" : "active"}</span></td>
          <td>${fmtTime(t.created_at)}</td>
          <td>
            <div class="row-actions">
              <button data-token-action="toggle-debug" data-id="${t.id}" data-enabled="${t.debug_enabled ? "0" : "1"}">
                Debug ${t.debug_enabled ? "off" : "on"}
              </button>
              <button data-token-action="revoke" data-id="${t.id}">Revoke</button>
            </div>
          </td>
        </tr>
      `;
    })
    .join("");

  el.tokensTable.innerHTML = `
    <table>
      <thead>
        <tr>
          <th>ID</th>
          <th>Label</th>
          <th>Tenant</th>
          <th>RPM</th>
          <th>TPD</th>
          <th>Debug</th>
          <th>Status</th>
          <th>Created</th>
          <th>Aktion</th>
        </tr>
      </thead>
      <tbody>${rows || '<tr><td colspan="9">Keine Tokens</td></tr>'}</tbody>
    </table>
  `;
}

function renderUsage(usage) {
  const rows = (usage.by_token || [])
    .map(
      (item) => `
        <tr>
          <td>${item.token_id}</td>
          <td>${item.requests_1m}</td>
          <td>${item.tokens_1m}</td>
          <td>${item.requests_24h}</td>
          <td>${item.tokens_24h}</td>
        </tr>
      `
    )
    .join("");

  el.usageTable.innerHTML = `
    <table>
      <thead>
        <tr>
          <th>Token ID</th>
          <th>Requests 1m</th>
          <th>Tokens 1m</th>
          <th>Requests 24h</th>
          <th>Tokens 24h</th>
        </tr>
      </thead>
      <tbody>${rows || '<tr><td colspan="5">Noch keine Nutzung</td></tr>'}</tbody>
    </table>
  `;
}

function renderLogs(logs) {
  const rows = (logs || [])
    .map(
      (item) => `
      <tr>
        <td>${item.id}</td>
        <td>${escapeHtml(item.request_id || "-")}</td>
        <td>${item.token_id}</td>
        <td>${escapeHtml(item.model_name || "-")}</td>
        <td>${item.worker_id ?? "-"}</td>
        <td>${item.total_tokens ?? 0}</td>
        <td>${item.duration_ms ?? 0}</td>
        <td>${item.http_status ?? "-"}</td>
        <td>${fmtTime(item.created_at)}</td>
      </tr>
    `
    )
    .join("");

  el.logsTable.innerHTML = `
    <table>
      <thead>
        <tr>
          <th>ID</th>
          <th>Request ID</th>
          <th>Token</th>
          <th>Model</th>
          <th>Worker</th>
          <th>Total Tokens</th>
          <th>Duration ms</th>
          <th>Status</th>
          <th>Zeit</th>
        </tr>
      </thead>
      <tbody>${rows || '<tr><td colspan="9">Keine Logs</td></tr>'}</tbody>
    </table>
  `;
}

async function loadLogs() {
  const params = new URLSearchParams();
  if (logFilters.token_id) params.set("token_id", logFilters.token_id);
  if (logFilters.model) params.set("model", logFilters.model);
  if (logFilters.limit) params.set("limit", logFilters.limit);

  const logsRes = await api(`/admin/requests?${params.toString()}`, { method: "GET" });
  renderLogs(logsRes.logs || []);
}

async function loadAll() {
  saveConfig();
  try {
    const [workersRes, tokensRes, usageRes] = await Promise.all([
      api("/admin/workers", { method: "GET" }),
      api("/admin/tokens", { method: "GET" }),
      api("/admin/usage/metrics", { method: "GET" }),
    ]);

    const workers = workersRes.workers || [];
    const tokens = tokensRes.tokens || [];
    renderWorkers(workers);
    renderTokens(tokens);
    renderUsage(usageRes || {});
    await loadLogs();

    const modelCount = new Set(workers.flatMap((w) => w.models || [])).size;
    el.statWorkers.textContent = String(workers.length);
    el.statModels.textContent = String(modelCount);
    el.statTokens.textContent = String(tokens.length);
    el.statReq1m.textContent = String(usageRes?.total?.requests_1m ?? 0);
    el.statTok1m.textContent = String(usageRes?.total?.tokens_1m ?? 0);
    el.statTok24h.textContent = String(usageRes?.total?.tokens_24h ?? 0);
  } catch (err) {
    el.workerMsg.textContent = `Laden fehlgeschlagen: ${err.message}`;
    el.tokenMsg.textContent = `Laden fehlgeschlagen: ${err.message}`;
  }
}

el.workerForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  el.workerMsg.textContent = "Arbeite...";
  const form = new FormData(el.workerForm);

  const payload = {
    name: form.get("name"),
    base_url: form.get("base_url"),
    api_key: form.get("api_key") || "",
    capacity_hint: Number(form.get("capacity_hint")) || 1,
  };

  const tenantRaw = String(form.get("tenant_id") || "").trim();
  if (tenantRaw !== "") {
    const tenantNum = Number(tenantRaw);
    if (Number.isFinite(tenantNum) && tenantNum > 0) {
      payload.tenant_id = tenantNum;
    }
  }

  try {
    const res = await api("/admin/workers", {
      method: "POST",
      body: JSON.stringify(payload),
    });
    el.workerMsg.textContent = `Worker ${res.worker_id} angelegt, Modelle: ${(res.models || []).join(", ")}`;
    await loadAll();
  } catch (err) {
    el.workerMsg.textContent = `Fehler: ${err.message}`;
  }
});

el.tokenForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  el.tokenMsg.textContent = "Arbeite...";
  const form = new FormData(el.tokenForm);

  const payload = {
    label: form.get("label"),
    tenant_id: Number(form.get("tenant_id")) || 1,
    debug_enabled: form.get("debug_enabled") === "on",
    quota_requests_per_min: asInputNumber(form.get("quota_requests_per_min")),
    quota_tokens_per_day: asInputNumber(form.get("quota_tokens_per_day")),
  };

  try {
    const res = await api("/admin/tokens", {
      method: "POST",
      body: JSON.stringify(payload),
    });
    el.tokenMsg.textContent = `Token erstellt: ${res.token}`;
    await loadAll();
  } catch (err) {
    el.tokenMsg.textContent = `Fehler: ${err.message}`;
  }
});

el.workersTable.addEventListener("click", async (event) => {
  const button = event.target.closest("button[data-action]");
  if (!button) return;
  const id = button.dataset.id;
  const action = button.dataset.action;

  try {
    if (action === "refresh") {
      await api(`/admin/workers/${id}/refresh`, { method: "POST" });
    } else if (action === "deactivate") {
      await api(`/admin/workers/${id}/deactivate`, { method: "POST" });
    }
    await loadAll();
  } catch (err) {
    el.workerMsg.textContent = `Aktion fehlgeschlagen: ${err.message}`;
  }
});

el.tokensTable.addEventListener("click", async (event) => {
  const button = event.target.closest("button[data-token-action]");
  if (!button) return;
  const id = button.dataset.id;
  const action = button.dataset.tokenAction;

  try {
    if (action === "revoke") {
      await api(`/admin/tokens/${id}/revoke`, { method: "POST" });
    } else if (action === "toggle-debug") {
      const enabled = button.dataset.enabled === "1";
      await api(`/admin/tokens/${id}/debug`, {
        method: "POST",
        body: JSON.stringify({ enabled }),
      });
    }
    await loadAll();
  } catch (err) {
    el.tokenMsg.textContent = `Aktion fehlgeschlagen: ${err.message}`;
  }
});

el.reloadBtn.addEventListener("click", () => loadAll());
el.logsFilterForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const form = new FormData(el.logsFilterForm);
  logFilters.token_id = String(form.get("token_id") || "").trim();
  logFilters.model = String(form.get("model") || "").trim();
  logFilters.limit = String(form.get("limit") || "100").trim();
  try {
    await loadLogs();
  } catch (err) {
    el.tokenMsg.textContent = `Log-Filter fehlgeschlagen: ${err.message}`;
  }
});
el.apiBase.addEventListener("change", saveConfig);
el.adminToken.addEventListener("change", saveConfig);

loadConfig();
loadAll();
