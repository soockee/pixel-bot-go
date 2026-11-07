"use strict";

const columns = 80;
const rows = 50;
const pathLimit = 600;
const recentSeconds = 60;
const buttonColors = { left: "#ffd27b", right: "#ff8f9d", middle: "#a6b7ff", x1: "#e7a8fb", x2: "#e7a8fb" };
const movement = new Uint32Array(columns * rows);
const buttons = Object.fromEntries(Object.keys(buttonColors).map(name => [name, new Uint32Array(columns * rows)]));
const activity = Array.from({ length: recentSeconds }, () => ({ second: -1, moves: 0, presses: 0 }));
const path = new Array(pathLimit);
const el = id => document.getElementById(id);
const map = el("map");
const timeline = el("timeline");
const camera = el("camera");
const recentInput = [];
const cameraPoints = new Array(pathLimit);
const buttonNames = { left: "Left", right: "Right", middle: "Middle", x1: "Mouse 4", x2: "Mouse 5" };
const heat = document.createElement("canvas");
heat.width = columns;
heat.height = rows;

let pathCount = 0;
let moves = 0;
let presses = 0;
let segments = 0;
let peakMovement = 0;
let elapsed = 0;
let clientWidth = 0;
let clientHeight = 0;
let active = false;
let sessionEnd = "";
let rendered = false;
let schema = 1;
let source = "manual";
const semantic = {
  cast_dispatched: { id: "casts", label: "Cast dispatched", count: 0 },
  bite_available: { id: "available-bites", label: "Bite available", count: 0 },
  bite_detected: { id: "detected-bites", label: "Bite detected", count: 0 },
  reel_attempt: { id: "reels", label: "Simulated reel attempt", count: 0 },
  reel_dispatched: { id: "reels", label: "Dry-run reel dispatched", count: 0 },
};
let pathEpoch = 0;
let clickCount = 0;
let dragCount = 0;
let rightDrags = 0;
let keyCount = 0;
let cameraActive = false;
let cameraCount = 0;
let cameraX = 0;
let cameraY = 0;

function logInput(event, label, detail) {
  recentInput.push([formatTime(Math.floor(event.t_us / 1_000_000)) +
    "." + String(Math.floor(event.t_us / 1000) % 1000).padStart(3, "0"), label, detail]);
  if (recentInput.length > 24) recentInput.shift();
}

function trackCamera(event) {
  if (event.type === "button_down" && event.button === "right") {
    cameraActive = true;
    cameraX = cameraY = 0;
    cameraCount = 1;
    cameraPoints[0] = { x: 0, y: 0 };
  } else if (event.type === "button_up" && event.button === "right" ||
      event.type === "mouse_gap" || event.type === "segment_end") {
    cameraActive = false;
  } else if (cameraActive && event.type === "move" &&
      (Number.isFinite(event.dx) || Number.isFinite(event.dy))) {
    cameraX += event.dx || 0;
    cameraY += event.dy || 0;
    cameraPoints[cameraCount++ % pathLimit] = { x: cameraX, y: cameraY };
  }
}

function ingest(event) {
  if (Number.isFinite(event.t_us)) elapsed = Math.max(elapsed, event.t_us);
  if (event.type === "session_start") {
    schema = event.schema || 1;
    source = event.source || "manual";
    if (source !== "manual") {
      for (const section of document.querySelectorAll("[data-manual]")) section.hidden = true;
      el("semantic-metrics").hidden = false;
      el("intro").textContent = `Review ${source.replaceAll("_", " ")} events from a shared playtest trace.`;
      el("input-description").textContent = "Recent events in trace order. Virtual clocks restart per run.";
    }
    return;
  }
  if (source !== "manual" && Object.hasOwn(semantic, event.type)) {
    const metric = semantic[event.type];
    metric.count++;
    const detail = [event.run_id, event.event_id == null ? "" : `event ${event.event_id}`, event.outcome].filter(Boolean);
    logInput(event, metric.label, detail.join(" · "));
    return;
  }
  trackCamera(event);
  if (event.type === "mouse_gap") {
    pathEpoch++;
    return;
  }
  if (event.type === "key_down" && event.key && !event.key.repeat) {
    keyCount++;
    logInput(event, event.key.chord || event.key.name, "Key press");
    return;
  }
  if (event.type === "drag" || event.type === "click" || event.type === "gesture_interrupted") {
    if (event.type === "drag") {
      dragCount++;
      if (event.button === "right") rightDrags++;
    } else if (event.type === "click") {
      clickCount++;
    }
    const kind = { drag: "drag", click: "click", gesture_interrupted: "interrupted hold" }[event.type];
    const duration = Math.round((event.gesture?.duration_us || 0) / 1000);
    const travel = Math.round(event.gesture?.raw_travel || 0);
    logInput(event, `${buttonNames[event.button] || event.button} ${kind}`, `${duration} ms · ${travel} raw counts traveled`);
  }
  if (event.type === "wheel" || event.type === "wheel_horizontal") {
    logInput(event, event.type === "wheel" ? "Wheel" : "Horizontal wheel", String(event.wheel));
    return;
  }
  if (event.type === "session_end") {
    sessionEnd = event.status === "complete" ? "Recording complete" : "Recording incomplete";
    active = false;
    return;
  }
  if (event.type === "segment_start") {
    segments++;
    active = true;
    return;
  }
  if (event.type === "segment_end") {
    active = false;
    return;
  }

  const point = event.position;
  if (!point || !Number.isFinite(event.t_us) || event.t_us < 0 ||
      ![point.x, point.y, point.width, point.height].every(Number.isFinite) ||
      point.width <= 0 || point.height <= 0 ||
      point.x < 0 || point.y < 0 || point.x >= point.width || point.y >= point.height) return;
  const mappedClick = event.type === "click" || schema === 1 && event.type === "button_down";
  if (event.type !== "move" && event.type !== "button_down" && !mappedClick) return;

  const x = point.x / point.width;
  const y = point.y / point.height;
  const cell = Math.floor(y * rows) * columns + Math.floor(x * columns);
  const second = Math.floor(event.t_us / 1_000_000);
  const slot = activity[second % recentSeconds];
  if (slot.second !== second) {
    slot.second = second;
    slot.moves = 0;
    slot.presses = 0;
  }
  clientWidth = point.width;
  clientHeight = point.height;
  if (event.type === "move") {
    peakMovement = Math.max(peakMovement, ++movement[cell]);
    path[pathCount++ % pathLimit] = { x, y, segment: event.segment, epoch: pathEpoch, width: point.width, height: point.height };
    moves++;
    slot.moves++;
  } else {
    if (event.type === "button_down") {
      presses++;
      slot.presses++;
    }
    if (mappedClick && Object.hasOwn(buttons, event.button)) {
      buttons[event.button][cell]++;
    }
  }
}

function surface(canvas) {
  const width = canvas.clientWidth;
  const height = canvas.clientHeight;
  const ratio = window.devicePixelRatio || 1;
  const pixelWidth = Math.max(1, Math.round(width * ratio));
  const pixelHeight = Math.max(1, Math.round(height * ratio));
  if (canvas.width !== pixelWidth || canvas.height !== pixelHeight) {
    canvas.width = pixelWidth;
    canvas.height = pixelHeight;
  }
  const ctx = canvas.getContext("2d");
  ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
  ctx.clearRect(0, 0, width, height);
  return { ctx, width, height };
}

function renderMap() {
  const { ctx, width, height } = surface(map);
  const aspect = clientWidth && clientHeight ? clientWidth / clientHeight : 16 / 9;
  const w = Math.min(width, height * aspect);
  const h = w / aspect;
  const left = (width - w) / 2;
  const top = (height - h) / 2;

  ctx.fillStyle = "#172b34";
  ctx.fillRect(left, top, w, h);
  ctx.strokeStyle = "#273f49";
  ctx.lineWidth = 1;
  ctx.beginPath();
  for (let i = 1; i < 4; i++) {
    ctx.moveTo(left + w * i / 4, top);
    ctx.lineTo(left + w * i / 4, top + h);
    ctx.moveTo(left, top + h * i / 4);
    ctx.lineTo(left + w, top + h * i / 4);
  }
  ctx.stroke();

  if (el("heat-layer").checked && peakMovement > 0) {
    const heatCtx = heat.getContext("2d");
    const image = heatCtx.createImageData(columns, rows);
    for (let i = 0; i < movement.length; i++) {
      if (!movement[i]) continue;
      const strength = Math.log1p(movement[i]) / Math.log1p(peakMovement);
      image.data.set([30 + 95 * strength, 151 + 94 * strength, 183 - 52 * strength, 40 + 200 * strength], 4 * i);
    }
    heatCtx.putImageData(image, 0, 0);
    ctx.drawImage(heat, left, top, w, h);
  }

  if (el("path-layer").checked) {
    ctx.strokeStyle = "#dbf6ff";
    ctx.lineWidth = 1.35;
    ctx.beginPath();
    let previous;
    for (let i = Math.max(0, pathCount - pathLimit); i < pathCount; i++) {
      const point = path[i % pathLimit];
      const x = left + point.x * w;
      const y = top + point.y * h;
      if (previous && previous.segment === point.segment && previous.epoch === point.epoch &&
          previous.width === point.width && previous.height === point.height) {
        ctx.lineTo(x, y);
      } else {
        ctx.moveTo(x, y);
      }
      previous = point;
    }
    ctx.stroke();
  }

  if (el("click-layer").checked) {
    for (const [button, cells] of Object.entries(buttons)) {
      ctx.fillStyle = buttonColors[button];
      ctx.strokeStyle = "#12202a";
      ctx.lineWidth = 1;
      for (let i = 0; i < cells.length; i++) {
        if (!cells[i]) continue;
        const x = left + (i % columns + 0.5) * w / columns;
        const y = top + (Math.floor(i / columns) + 0.5) * h / rows;
        ctx.beginPath();
        ctx.arc(x, y, Math.min(13, 3 + 2 * Math.log2(cells[i])), 0, 2 * Math.PI);
        ctx.fill();
        ctx.stroke();
      }
    }
  }

  if (!moves && !presses) {
    ctx.fillStyle = "#cfdee6";
    ctx.textAlign = "center";
    ctx.textBaseline = "middle";
    ctx.font = "14px Segoe UI, sans-serif";
    ctx.fillText("Waiting for mouse events", width / 2, height / 2);
  }
}

function renderCamera() {
  const { ctx, width, height } = surface(camera);
  ctx.fillStyle = "#cfdee6";
  ctx.font = "14px Segoe UI, sans-serif";
  ctx.textAlign = "center";
  if (cameraCount < 2) {
    ctx.fillText("Waiting for relative movement while the right button is held", width / 2, height / 2);
    return;
  }
  const points = [];
  for (let i = Math.max(0, cameraCount - pathLimit); i < cameraCount; i++) {
    points.push(cameraPoints[i % pathLimit]);
  }
  const xs = points.map(point => point.x);
  const ys = points.map(point => point.y);
  const minX = Math.min(0, ...xs);
  const maxX = Math.max(0, ...xs);
  const minY = Math.min(0, ...ys);
  const maxY = Math.max(0, ...ys);
  const scale = Math.min((width - 30) / Math.max(1, maxX - minX), (height - 30) / Math.max(1, maxY - minY));
  const x = raw => width / 2 + (raw - (minX + maxX) / 2) * scale;
  const y = raw => height / 2 + (raw - (minY + maxY) / 2) * scale;
  ctx.strokeStyle = "#273f49";
  ctx.beginPath();
  ctx.moveTo(x(0), 0);
  ctx.lineTo(x(0), height);
  ctx.moveTo(0, y(0));
  ctx.lineTo(width, y(0));
  ctx.stroke();
  ctx.strokeStyle = "#ff8f9d";
  ctx.lineWidth = 2;
  ctx.beginPath();
  points.forEach((point, index) => index ? ctx.lineTo(x(point.x), y(point.y)) : ctx.moveTo(x(point.x), y(point.y)));
  ctx.stroke();
  ctx.fillStyle = "#ffd27b";
  ctx.beginPath();
  ctx.arc(x(cameraX), y(cameraY), 3, 0, 2 * Math.PI);
  ctx.fill();
}

function renderInputLog() {
  const rows = recentInput.slice().reverse().map(fields => {
    const row = document.createElement("tr");
    for (const field of fields) {
      const cell = document.createElement("td");
      cell.textContent = field;
      row.append(cell);
    }
    return row;
  });
  el("input-log").replaceChildren(...rows);
}

function renderTimeline() {
  const { ctx, width, height } = surface(timeline);
  const last = Math.floor(elapsed / 1_000_000);
  const first = Math.max(0, last - recentSeconds + 1);
  const span = recentSeconds;
  const cell = width / span;
  const visible = activity.filter(entry => entry.second >= first && entry.second <= last);
  const peak = Math.max(1, ...visible.map(entry => entry.moves));
  const clickPeak = Math.max(1, ...visible.map(entry => entry.presses));
  const bottom = height - 15;
  for (const entry of visible) {
    const x = (entry.second - first) * cell;
    const barHeight = entry.moves / peak * (bottom - 12);
    ctx.fillStyle = "#53cbb0";
    ctx.fillRect(x, bottom - barHeight, Math.max(1, cell - 2), barHeight);
    if (entry.presses) {
      ctx.fillStyle = "#ff8f9d";
      ctx.beginPath();
      ctx.arc(x + cell / 2, bottom - 5 - entry.presses / clickPeak * (bottom - 18), 3, 0, 2 * Math.PI);
      ctx.fill();
    }
  }
  ctx.fillStyle = "#b2c3cc";
  ctx.font = "11px Segoe UI, sans-serif";
  ctx.fillText(formatTime(first), 6, height - 3);
  ctx.textAlign = "right";
  ctx.fillText(formatTime(first + span - 1), width - 6, height - 3);
  ctx.textAlign = "left";
}

function formatTime(seconds) {
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`;
}

function render() {
  rendered = false;
  el("moves").textContent = moves.toLocaleString();
  el("presses").textContent = presses.toLocaleString();
  el("segments").textContent = segments.toLocaleString();
  el("duration").textContent = formatTime(Math.floor(elapsed / 1_000_000));
  if (clientWidth) el("dimensions").textContent = `Latest client area: ${clientWidth} × ${clientHeight} px`;
  el("clicks").textContent = schema === 1 ? "—" : clickCount.toLocaleString();
  el("drags").textContent = schema === 1 ? "—" : dragCount.toLocaleString();
  el("right-drags").textContent = schema === 1 ? "—" : rightDrags.toLocaleString();
  el("keys").textContent = schema === 1 ? "—" : keyCount.toLocaleString();
  if (source === "manual") {
    renderMap();
    renderCamera();
    renderTimeline();
  } else {
    const totals = {};
    for (const { id, count } of Object.values(semantic)) totals[id] = (totals[id] || 0) + count;
    for (const [id, count] of Object.entries(totals)) el(id).textContent = count.toLocaleString();
  }
  renderInputLog();
}

function requestRender() {
  if (!rendered) {
    rendered = true;
    requestAnimationFrame(render);
  }
}

function refreshFrame() {
  const frame = el("frame");
  frame.onload = () => { el("frame-panel").hidden = false; };
  frame.src = `/api/frame?t=${Date.now()}`;
}

async function load() {
  let offset = 0;
  while (true) {
    try {
      const response = await fetch(`/api/events?offset=${offset}`);
      if (!response.ok) throw new Error(await response.text());
      const page = await response.json();
      page.events.forEach(ingest);
      offset = page.next_offset;
      if (page.events.length) requestRender();
      if (page.more) continue;
      el("status").textContent = sessionEnd || (page.live
        ? (active ? "Recording" : "Waiting for selected window")
        : "Saved trace · no session footer");
      refreshFrame();
      if (!page.live || sessionEnd) return;
    } catch (error) {
      el("status").textContent = `Dashboard unavailable: ${error.message.trim()}`;
    }
    await new Promise(resolve => setTimeout(resolve, 400));
  }
}

for (const id of ["heat-layer", "click-layer", "path-layer"]) {
  el(id).addEventListener("change", requestRender);
}
const resize = new ResizeObserver(requestRender);
resize.observe(map);
resize.observe(timeline);
resize.observe(camera);
requestRender();
load();
