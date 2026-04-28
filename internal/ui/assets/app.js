const terminalElement = document.getElementById("terminal");
const term = new Terminal({
  theme: { background: "#0a0a0a", foreground: "#d4d4d4" },
  fontFamily: "SFMono-Regular, Menlo, Monaco, Consolas, monospace",
  fontSize: 12,
  cursorBlink: true,
  scrollback: 2000,
});
term.open(terminalElement);
term.writeln("\x1b[90mInfraSage UI ready.\x1b[0m");

const promptInput = document.getElementById("promptInput");
const promptSend = document.getElementById("promptSend");
const driftCheck = document.getElementById("driftCheck");
const remediateBtn = document.getElementById("remediateBtn");
const driftOutput = document.getElementById("driftOutput");
const modeButtons = document.querySelectorAll(".mode-btn");
const modeStatus = document.getElementById("modeStatus");

let currentMode = "github";
let awsAvailable = false;

// ── Busy state ──────────────────────────────────────────────────────────────
function setBusy(busy) {
  promptSend.disabled = busy;
  driftCheck.disabled = busy;
  scanBtn.disabled = busy;
  deployBtn.disabled = busy;
  promptSend.textContent = busy ? "Running…" : "Run";
  promptSend.classList.toggle("busy", busy);
  if (busy) {
    resetPipeline();
    resetScans();
  }
}

// ── Pipeline ─────────────────────────────────────────────────────────────────
function resetPipeline() {
  document.querySelectorAll(".pipeline-node").forEach((node) => {
    node.classList.remove("active");
  });
}

function updatePipeline(step) {
  document.querySelectorAll(".pipeline-node").forEach((node) => {
    if (node.dataset.step === step) {
      node.classList.add("active");
    }
  });
}

// ── Scan dashboard ───────────────────────────────────────────────────────────
function resetScans() {
  document.querySelectorAll(".scan-column").forEach((col) => {
    const statusEl = col.querySelector("[data-scan-status]");
    statusEl.textContent = "Idle";
    statusEl.removeAttribute("data-status");
    col.querySelector("[data-scan-counts]").textContent = "0 passed / 0 failed";
    col.querySelector("[data-scan-findings]").innerHTML = "";
  });
}

function updateScan(payload) {
  const column = document.querySelector(`[data-scan="${payload.tool}"]`);
  if (!column) return;

  const statusEl = column.querySelector("[data-scan-status]");
  statusEl.textContent = payload.status;
  statusEl.setAttribute("data-status", payload.status);

  column.querySelector(
    "[data-scan-counts]"
  ).textContent = `${payload.passed} passed / ${payload.failed} failed`;

  const list = column.querySelector("[data-scan-findings]");
  list.innerHTML = "";
  (payload.findings || []).slice(0, 5).forEach((finding) => {
    const li = document.createElement("li");
    const badge = document.createElement("span");
    const severity = (finding.severity || "LOW").toLowerCase();
    badge.className = `scan-badge ${severity}`;
    badge.textContent = finding.severity || "LOW";
    li.appendChild(badge);
    li.append(`${finding.id || ""} ${finding.message || finding.resource || ""}`);
    list.appendChild(li);
  });
}

// ── Drift ────────────────────────────────────────────────────────────────────
function updateDrift(payload) {
  driftOutput.textContent = payload.output || "";
  remediateBtn.hidden = payload.status !== "drift";
}

// ── Mode ─────────────────────────────────────────────────────────────────────
function setMode(mode) {
  currentMode = mode;
  modeButtons.forEach((btn) => {
    const isActive = btn.dataset.mode === mode;
    btn.classList.toggle("active", isActive);
    if (btn.dataset.mode === "aws") {
      btn.disabled = !awsAvailable;
    }
  });
  modeStatus.textContent = mode === "aws" ? "Real deployment" : "Dry-run";
}

// ── Toast notification ───────────────────────────────────────────────────────
function showToast(msg) {
  const existing = document.querySelector(".toast");
  if (existing) existing.remove();
  const t = document.createElement("div");
  t.className = "toast";
  t.textContent = msg;
  document.body.appendChild(t);
  setTimeout(() => {
    t.classList.add("toast-fade");
    setTimeout(() => t.remove(), 300);
  }, 2200);
}

// ── Config / Init ─────────────────────────────────────────────────────────────
async function fetchConfig() {
  const res = await fetch("/api/config");
  const cfg = await res.json();
  awsAvailable = cfg.awsAvailable;
  setMode(cfg.mode);
  const grafanaFrame = document.getElementById("grafanaFrame");
  grafanaFrame.src = cfg.grafanaUrl;
}

async function postJSON(url, body) {
  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body || {}),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || "Request failed");
  }
  return res.json();
}

// ── Button handlers ──────────────────────────────────────────────────────────
const scanBtn = document.getElementById("scanBtn");
const scanFileInput = document.getElementById("scanFileInput");
const deployBtn = document.getElementById("deployBtn");
const deployFileInput = document.getElementById("deployFileInput");

promptSend.addEventListener("click", async () => {
  const prompt = promptInput.value.trim();
  if (!prompt) return;
  promptInput.value = "";
  try {
    await postJSON("/api/ask", { prompt });
  } catch (err) {
    term.writeln(`\x1b[31m${err.message}\x1b[0m`);
  }
});

promptInput.addEventListener("keydown", (event) => {
  if (event.key === "Enter") {
    promptSend.click();
  }
});

driftCheck.addEventListener("click", async () => {
  try {
    await postJSON("/api/drift", {});
  } catch (err) {
    term.writeln(`\x1b[31m${err.message}\x1b[0m`);
  }
});

remediateBtn.addEventListener("click", async () => {
  try {
    await postJSON("/api/remediate", {});
  } catch (err) {
    term.writeln(`\x1b[31m${err.message}\x1b[0m`);
  }
});

modeButtons.forEach((btn) => {
  btn.addEventListener("click", async () => {
    try {
      const response = await postJSON("/api/mode", { mode: btn.dataset.mode });
      setMode(response.mode);
      const label = response.mode === "aws" ? "AWS — Real deployment" : "GitHub Actions — Dry-run";
      showToast(`Mode switched to ${label}`);
    } catch (err) {
      term.writeln(`\x1b[31m${err.message}\x1b[0m`);
    }
  });
});

scanBtn.addEventListener("click", async () => {
  const file = scanFileInput.value.trim();
  try {
    await postJSON("/api/scan", { file });
  } catch (err) {
    term.writeln(`\x1b[31m${err.message}\x1b[0m`);
  }
});

deployBtn.addEventListener("click", async () => {
  const file = deployFileInput.value.trim();
  try {
    await postJSON("/api/deploy", { file });
  } catch (err) {
    term.writeln(`\x1b[31m${err.message}\x1b[0m`);
  }
});

// ── Live metrics panel ───────────────────────────────────────────────────────
function updateMetrics(data) {
  const fmt = (v) => (typeof v === "number" ? (Number.isInteger(v) ? v : v.toFixed(2)) : "—");
  const el = (id) => document.getElementById(id);
  el("metricGenerations").textContent = fmt(data.generationsTotal);
  el("metricFindings").textContent = fmt(data.scanFindings);
  el("metricDrift").textContent = fmt(data.driftTotal);
  el("metricLatency").textContent = data.modelLatencyP50
    ? fmt(data.modelLatencyP50)
    : "—";
}

async function pollMetrics() {
  try {
    const res = await fetch("/api/metrics");
    if (res.ok) {
      const data = await res.json();
      updateMetrics(data);
    }
  } catch (_) {
    // metrics server not running — keep showing dashes
  }
}

// WebSocket ────────────────────────────────────────────────────────────────
function connectWebSocket() {
  const protocol = window.location.protocol === "https:" ? "wss" : "ws";
  const ws = new WebSocket(`${protocol}://${window.location.host}/ws/stream`);
  ws.onmessage = (event) => {
    try {
      const message = JSON.parse(event.data);
      if (message.type === "terminal") {
        if (message.payload.level === "error") {
          term.writeln(`\x1b[31m${message.payload.line}\x1b[0m`);
        } else {
          term.writeln(message.payload.line);
        }
      }
      if (message.type === "pipeline") {
        updatePipeline(message.payload.step);
      }
      if (message.type === "scan") {
        updateScan(message.payload);
      }
      if (message.type === "mode") {
        awsAvailable = message.payload.awsAvailable;
        setMode(message.payload.current);
      }
      if (message.type === "drift") {
        updateDrift(message.payload);
      }
      if (message.type === "busy") {
        setBusy(message.payload.busy);
      }
      if (message.type === "deploy") {
        term.writeln(
          `\x1b[32m✅ PR opened →\x1b[0m \x1b[36m\x1b[4m${message.payload.url}\x1b[0m`
        );
      }
    } catch (err) {
      console.error("WS parse error", err);
    }
  };

  ws.onclose = () => {
    setTimeout(connectWebSocket, 1500);
  };
}

fetchConfig();
connectWebSocket();
pollMetrics();
setInterval(pollMetrics, 30_000);
