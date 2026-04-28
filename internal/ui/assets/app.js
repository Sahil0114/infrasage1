const terminalElement = document.getElementById("terminal");
const term = new Terminal({
  theme: { background: "#0a0a0a", foreground: "#ffffff" },
  fontFamily: "SFMono-Regular, Menlo, Monaco, Consolas, monospace",
  fontSize: 12,
  cursorBlink: true,
});
term.open(terminalElement);
term.writeln("InfraSage UI ready.");

const promptInput = document.getElementById("promptInput");
const promptSend = document.getElementById("promptSend");
const driftCheck = document.getElementById("driftCheck");
const remediateBtn = document.getElementById("remediateBtn");
const driftOutput = document.getElementById("driftOutput");
const modeButtons = document.querySelectorAll(".mode-btn");
const modeStatus = document.getElementById("modeStatus");

let currentMode = "github";
let awsAvailable = false;

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

function updatePipeline(step) {
  document.querySelectorAll(".pipeline-node").forEach((node) => {
    if (node.dataset.step === step) {
      node.classList.add("active");
    }
  });
}

function updateScan(payload) {
  const column = document.querySelector(`[data-scan="${payload.tool}"]`);
  if (!column) return;

  column.querySelector("[data-scan-status]").textContent = payload.status;
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

function updateDrift(payload) {
  driftOutput.textContent = payload.output || "";
  remediateBtn.hidden = payload.status !== "drift";
}

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
    } catch (err) {
      term.writeln(`\x1b[31m${err.message}\x1b[0m`);
    }
  });
});

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
      if (message.type === "deploy") {
        term.writeln(`PR opened: ${message.payload.url}`);
      }
    } catch (err) {
      console.error("WS error", err);
    }
  };

  ws.onclose = () => {
    setTimeout(connectWebSocket, 1500);
  };
}

fetchConfig();
connectWebSocket();
