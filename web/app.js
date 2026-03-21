const state = {
  activeTab: "traces",
  liveCount: 0,
  traceSort: { field: "time", direction: "desc" },
  traces: [],
  traceDetail: null,
  logs: [],
  services: null,
  errors: [],
  queries: [],
  socket: null,
};

const servicePalette = ["#ffd166", "#7bdff2", "#7ae582", "#f4a261", "#ef476f", "#cdb4db", "#84dcc6"];

function serviceColor(service) {
  let hash = 0;
  for (let index = 0; index < service.length; index += 1) {
    hash = (hash * 31 + service.charCodeAt(index)) >>> 0;
  }
  return servicePalette[hash % servicePalette.length];
}

async function fetchJSON(path) {
  const response = await fetch(path);
  if (!response.ok) {
    throw new Error(`Request failed for ${path}: ${response.status}`);
  }
  return response.json();
}

function formatTime(nanos) {
  if (!nanos) {
    return "n/a";
  }
  return new Date(nanos / 1e6).toLocaleTimeString();
}

function formatDuration(nanos) {
  if (!Number.isFinite(nanos)) {
    return "0ms";
  }
  if (nanos >= 1e9) {
    return `${(nanos / 1e9).toFixed(2)}s`;
  }
  if (nanos >= 1e6) {
    return `${(nanos / 1e6).toFixed(1)}ms`;
  }
  return `${Math.round(nanos / 1e3)}us`;
}

function safeJSON(value) {
  try {
    return JSON.stringify(typeof value === "string" ? JSON.parse(value) : value, null, 2);
  } catch {
    return String(value);
  }
}

function setActiveTab(nextTab) {
  state.activeTab = nextTab;
  document.querySelectorAll(".tab").forEach((tab) => {
    tab.classList.toggle("is-active", tab.dataset.tab === nextTab);
  });
  document.querySelectorAll(".tab-panel").forEach((panel) => {
    panel.classList.toggle("is-hidden", panel.dataset.panel !== nextTab);
  });
  document.querySelectorAll("[data-filters]").forEach((panel) => {
    panel.classList.toggle("is-hidden", panel.dataset.filters !== nextTab);
  });
}

function renderEmpty(target, message, columns = 1) {
  target.innerHTML = `<tr><td class="empty" colspan="${columns}">${message}</td></tr>`;
}

function renderTraces() {
  const target = document.querySelector("#traces-table");
  if (!state.traces.length) {
    renderEmpty(target, "No traces yet. Point your OTEL exporter at localhost:4317.", 5);
    return;
  }

  const traces = [...state.traces].sort(compareTraces);
  target.innerHTML = traces.map((trace) => `
    <tr class="is-clickable" data-trace-id="${trace.traceId}">
      <td>${trace.serviceNames.map((service) => `<span class="badge service">${service}</span>`).join(" ")}</td>
      <td>${trace.name || trace.traceId}</td>
      <td>${formatDuration(trace.durationNanos)}</td>
      <td>${trace.spanCount}</td>
      <td>${formatTime(trace.startTimeUnix)}</td>
    </tr>
  `).join("");

  target.querySelectorAll("[data-trace-id]").forEach((row) => {
    row.addEventListener("click", () => loadTraceDetail(row.dataset.traceId));
  });
}

function flattenTree(nodes, depth = 0, output = []) {
  nodes.forEach((node) => {
    output.push({ node, depth });
    flattenTree(node.children, depth + 1, output);
  });
  return output;
}

function renderTraceDetail() {
  const title = document.querySelector("#trace-detail-title");
  const waterfall = document.querySelector("#waterfall");
  const spanDetail = document.querySelector("#span-detail");

  if (!state.traceDetail) {
    title.textContent = "Select a trace";
    spanDetail.textContent = "Select a span to inspect its payload.";
    waterfall.innerHTML = "";
    return;
  }

  title.textContent = `${state.traceDetail.traceId} · ${state.traceDetail.spans.length} spans`;
  const flattened = flattenTree(state.traceDetail.tree);
  const start = Math.min(...state.traceDetail.spans.map((span) => span.startTimeUnix));
  const end = Math.max(...state.traceDetail.spans.map((span) => span.endTimeUnix));
  const total = Math.max(end - start, 1);
  const rowHeight = 42;
  const left = 220;
  const width = 720;
  const height = Math.max(240, flattened.length * rowHeight + 40);
  waterfall.setAttribute("viewBox", `0 0 1000 ${height}`);

  const bars = flattened.map(({ node, depth }, index) => {
    const x = left + ((node.span.startTimeUnix - start) / total) * width;
    const barWidth = Math.max(8, ((node.span.endTimeUnix - node.span.startTimeUnix) / total) * width);
    const y = 28 + index * rowHeight;
    const fill = serviceColor(node.span.serviceName || "unknown");
    return `
      <text x="${24 + depth * 18}" y="${y + 18}" fill="#f3f6f8" font-size="13">${node.span.name}</text>
      <rect class="span-bar" data-span-id="${node.span.spanId}" x="${x}" y="${y}" rx="8" ry="8" width="${barWidth}" height="22" fill="${fill}" opacity="0.86"></rect>
      <text x="${x + 8}" y="${y + 15}" fill="#111417" font-size="12">${node.span.serviceName}</text>
    `;
  }).join("");

  waterfall.innerHTML = `
    <defs>
      <pattern id="waterfall-grid" width="80" height="40" patternUnits="userSpaceOnUse">
        <path d="M 80 0 L 0 0 0 40" fill="none" stroke="rgba(161,184,207,0.08)" stroke-width="1"></path>
      </pattern>
    </defs>
    <rect width="1000" height="${height}" fill="url(#waterfall-grid)"></rect>
    ${bars}
  `;

  waterfall.querySelectorAll(".span-bar").forEach((bar) => {
    bar.addEventListener("click", () => {
      const span = state.traceDetail.spans.find((item) => item.spanId === bar.dataset.spanId);
      spanDetail.textContent = safeJSON(span);
    });
  });
}

function severityBadge(record) {
  const level = (record.severityText || "").toUpperCase();
  if (level.includes("ERROR") || record.severityNum >= 17) {
    return "error";
  }
  if (level.includes("WARN") || record.severityNum >= 13) {
    return "warn";
  }
  if (level.includes("INFO") || record.severityNum >= 9) {
    return "info";
  }
  return "debug";
}

function renderLogs() {
  const target = document.querySelector("#logs-table");
  if (!state.logs.length) {
    renderEmpty(target, "No matching logs.", 5);
    return;
  }

  target.innerHTML = state.logs.map((record) => `
    <tr>
      <td>${formatTime(record.timestamp)}</td>
      <td><span class="badge service">${record.serviceName || "unknown"}</span></td>
      <td><span class="badge ${severityBadge(record)}">${record.severityText || record.severityNum}</span></td>
      <td>${record.body}</td>
      <td><button class="tab badge trace" data-open-trace="${record.traceId}" type="button">${record.traceId || "n/a"}</button></td>
    </tr>
  `).join("");

  target.querySelectorAll("[data-open-trace]").forEach((button) => {
    button.addEventListener("click", () => {
      setActiveTab("traces");
      loadTraceDetail(button.dataset.openTrace);
    });
  });
}

function renderServices() {
  const svg = document.querySelector("#services-graph");
  if (!state.services || !state.services.services.length) {
    svg.innerHTML = "";
    return;
  }

  const spacing = 920 / Math.max(state.services.services.length, 1);
  const positions = new Map();
  state.services.services.forEach((service, index) => {
    positions.set(service, { x: 60 + spacing * index, y: 240 + ((index % 2 === 0) ? -90 : 90) });
  });

  const edges = state.services.edges.map((edge) => {
    const from = positions.get(edge.from);
    const to = positions.get(edge.to);
    const color = edge.errorRate > 5 ? "#ef476f" : edge.errorRate >= 1 ? "#f4a261" : "#7ae582";
    return `
      <line x1="${from.x}" y1="${from.y}" x2="${to.x}" y2="${to.y}" stroke="${color}" stroke-width="4" marker-end="url(#arrow)"></line>
      <text x="${(from.x + to.x) / 2}" y="${(from.y + to.y) / 2 - 10}" fill="#f3f6f8" font-size="13">${edge.count} calls · ${edge.errorRate.toFixed(1)}%</text>
    `;
  }).join("");

  const nodes = state.services.services.map((service) => {
    const position = positions.get(service);
    const color = serviceColor(service);
    return `
      <circle cx="${position.x}" cy="${position.y}" r="52" fill="${color}" opacity="0.22"></circle>
      <circle cx="${position.x}" cy="${position.y}" r="40" fill="#111417" stroke="${color}" stroke-width="3"></circle>
      <text x="${position.x}" y="${position.y + 4}" fill="#f3f6f8" text-anchor="middle" font-size="15">${service}</text>
    `;
  }).join("");

  svg.innerHTML = `
    <defs>
      <marker id="arrow" markerWidth="10" markerHeight="10" refX="7" refY="3" orient="auto">
        <path d="M0,0 L0,6 L9,3 z" fill="#f3f6f8"></path>
      </marker>
    </defs>
    ${edges}
    ${nodes}
  `;
}

function renderErrors() {
  const target = document.querySelector("#errors-table");
  if (!state.errors.length) {
    renderEmpty(target, "No grouped errors.", 6);
    return;
  }

  target.innerHTML = state.errors.map((group) => `
    <tr class="is-clickable" data-error-id="${group.fingerprint}">
      <td>${group.excType}</td>
      <td>${group.messageNorm}</td>
      <td>${group.count}</td>
      <td>${formatTime(group.firstSeen)}</td>
      <td>${formatTime(group.lastSeen)}</td>
      <td>${group.services.map((service) => `<span class="badge service">${service}</span>`).join(" ")}</td>
    </tr>
  `).join("");

  target.querySelectorAll("[data-error-id]").forEach((row) => {
    row.addEventListener("click", () => loadErrorDetail(row.dataset.errorId));
  });
}

function compareTraces(left, right) {
  let compare = 0;
  switch (state.traceSort.field) {
    case "service":
      compare = (left.serviceNames[0] || "").localeCompare(right.serviceNames[0] || "");
      break;
    case "name":
      compare = (left.name || left.traceId).localeCompare(right.name || right.traceId);
      break;
    case "duration":
      compare = left.durationNanos - right.durationNanos;
      break;
    case "spans":
      compare = left.spanCount - right.spanCount;
      break;
    case "time":
    default:
      compare = left.startTimeUnix - right.startTimeUnix;
      break;
  }
  return state.traceSort.direction === "asc" ? compare : compare * -1;
}

function selectTraceSort(field) {
  if (state.traceSort.field === field) {
    state.traceSort.direction = state.traceSort.direction === "asc" ? "desc" : "asc";
  } else {
    state.traceSort.field = field;
    state.traceSort.direction = field === "time" ? "desc" : "asc";
  }
  renderTraces();
}

function parseEventPayload(sampleSpan) {
  if (!sampleSpan || !sampleSpan.events) {
    return [];
  }

  try {
    return JSON.parse(sampleSpan.events);
  } catch {
    return [];
  }
}

function formatErrorDetail(detail) {
  if (!detail) {
    return "No error detail available.";
  }

  const sections = [];
  if (detail.group) {
    sections.push(`Exception: ${detail.group.excType || "unknown"}`);
    sections.push(`Fingerprint: ${detail.group.fingerprint}`);
    sections.push(`Message: ${detail.group.messageNorm || "n/a"}`);
    sections.push(`Count: ${detail.group.count}`);
    sections.push(`Services: ${(detail.group.services || []).join(", ") || "n/a"}`);
  }

  const events = parseEventPayload(detail.sampleSpan);
  const exceptionEvent = events.find((event) => event.name === "exception") || {};
  const attributes = exceptionEvent.attributes || {};
  const stack = attributes["exception.stacktrace"];

  if (detail.sampleSpan) {
    sections.push("");
    sections.push(`Sample span: ${detail.sampleSpan.name} (${detail.sampleSpan.spanId})`);
    if (attributes["exception.message"]) {
      sections.push(`Sample message: ${attributes["exception.message"]}`);
    }
    if (stack) {
      sections.push("");
      sections.push("Stack trace:");
      sections.push(stack);
    } else {
      sections.push("");
      sections.push("Sample payload:");
      sections.push(safeJSON(detail.sampleSpan));
    }
  }

  return sections.join("\n");
}

function updateServiceOptions(services) {
  const traceSelect = document.querySelector("#trace-service");
  const logSelect = document.querySelector("#log-service");
  const currentTrace = traceSelect.value;
  const currentLog = logSelect.value;
  const options = [`<option value="">All services</option>`]
    .concat((services || []).map((service) => `<option value="${service}">${service}</option>`))
    .join("");
  traceSelect.innerHTML = options;
  logSelect.innerHTML = options;
  traceSelect.value = currentTrace;
  logSelect.value = currentLog;
}

function renderQueries() {
  const target = document.querySelector("#queries-table");
  if (!state.queries.length) {
    renderEmpty(target, "No database spans detected yet.", 5);
    return;
  }

  target.innerHTML = state.queries.map((query) => `
    <tr>
      <td>${query.normalized}</td>
      <td>${query.count}</td>
      <td>${formatDuration(query.p95Latency)}</td>
      <td>${formatDuration(query.p99Latency)}</td>
      <td>${formatDuration(query.maxDuration)}</td>
    </tr>
  `).join("");
}

async function loadTraces() {
  const service = document.querySelector("#trace-service").value.trim();
  const minDuration = document.querySelector("#trace-duration").value.trim();
  const hasError = document.querySelector("#trace-errors").checked;
  const params = new URLSearchParams();
  if (service) params.set("service", service);
  if (minDuration) params.set("minDuration", minDuration);
  if (hasError) params.set("hasError", "true");
  state.traces = await fetchJSON(`/api/traces?${params.toString()}`);
  renderTraces();
  if (!state.traceDetail && state.traces.length) {
    await loadTraceDetail(state.traces[0].traceId);
  }
}

async function loadTraceDetail(traceId) {
  if (!traceId) {
    return;
  }
  state.traceDetail = await fetchJSON(`/api/traces/${encodeURIComponent(traceId)}`);
  renderTraceDetail();
}

async function loadLogs() {
  const service = document.querySelector("#log-service").value.trim();
  const severity = document.querySelector("#log-severity").value;
  const search = document.querySelector("#log-search").value.trim();
  const params = new URLSearchParams();
  if (service) params.set("service", service);
  if (severity) params.set("severity", severity);
  if (search) params.set("search", search);
  state.logs = await fetchJSON(`/api/logs?${params.toString()}`);
  renderLogs();
}

async function loadServices() {
  state.services = await fetchJSON("/api/services");
  updateServiceOptions(state.services.services);
  renderServices();
}

async function loadErrors() {
  state.errors = await fetchJSON("/api/errors");
  renderErrors();
}

async function loadErrorDetail(fingerprint) {
  const detail = await fetchJSON(`/api/errors/${encodeURIComponent(fingerprint)}`);
  document.querySelector("#error-detail").textContent = formatErrorDetail(detail);
}

async function loadQueries() {
  state.queries = await fetchJSON("/api/queries");
  renderQueries();
}

function connectStream() {
  const scheme = window.location.protocol === "https:" ? "wss" : "ws";
  const socket = new WebSocket(`${scheme}://${window.location.host}/api/stream`);
  state.socket = socket;

  socket.addEventListener("message", async () => {
    state.liveCount += 1;
    document.querySelector("#live-count").textContent = String(state.liveCount);
    try {
      await Promise.all([loadTraces(), loadLogs(), loadServices(), loadErrors(), loadQueries()]);
    } catch (error) {
      console.error(error);
    }
  });

  socket.addEventListener("close", () => {
    window.setTimeout(connectStream, 1200);
  });
}

async function bootstrap() {
  document.querySelectorAll(".tab").forEach((button) => {
    button.addEventListener("click", () => setActiveTab(button.dataset.tab));
  });
  document.querySelector("#refresh-traces").addEventListener("click", () => {
    loadTraces().catch(console.error);
  });
  document.querySelector("#refresh-logs").addEventListener("click", () => {
    loadLogs().catch(console.error);
  });
  document.querySelectorAll("[data-trace-sort]").forEach((button) => {
    button.addEventListener("click", () => selectTraceSort(button.dataset.traceSort));
  });

  await Promise.all([loadTraces(), loadLogs(), loadServices(), loadErrors(), loadQueries()]);
  connectStream();
}

bootstrap().catch((error) => {
  console.error(error);
});
