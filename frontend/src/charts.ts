// Canvas drawings for the Recorder tab, coloured from the theme tokens.
import { css } from "./dom";
import { BUTTONS, COLUMNS, RECENT_SECONDS, ROWS, formatSeconds, type Button, type Trace } from "./trace";

/** Click colours: left carries the accent, the others are quiet steps. */
export function buttonColors(): Record<Button, string> {
  return { left: css("--accent"), right: css("--bad"), middle: css("--text-2"), x1: css("--text-3"), x2: css("--text-3") };
}

function surface(canvas: HTMLCanvasElement) {
  const width = canvas.clientWidth;
  const height = canvas.clientHeight;
  const ratio = window.devicePixelRatio || 1;
  const pw = Math.max(1, Math.round(width * ratio));
  const ph = Math.max(1, Math.round(height * ratio));
  if (canvas.width !== pw || canvas.height !== ph) {
    canvas.width = pw;
    canvas.height = ph;
  }
  const ctx = canvas.getContext("2d")!;
  ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
  ctx.clearRect(0, 0, width, height);
  return { ctx, width, height };
}

function waiting(ctx: CanvasRenderingContext2D, width: number, height: number, text: string) {
  ctx.fillStyle = css("--text-3");
  ctx.font = `12px ${css("--font")}`;
  ctx.textAlign = "center";
  ctx.textBaseline = "middle";
  ctx.fillText(text, width / 2, height / 2);
}

const heat = Object.assign(document.createElement("canvas"), { width: COLUMNS, height: ROWS });

export interface MapLayers { heat: boolean; clicks: boolean; path: boolean }

/** drawMap draws the client area with movement heat, clicks and the recent path. */
export function drawMap(canvas: HTMLCanvasElement, t: Trace, layers: MapLayers): void {
  const { ctx, width, height } = surface(canvas);
  const aspect = t.clientWidth && t.clientHeight ? t.clientWidth / t.clientHeight : 16 / 9;
  const w = Math.min(width, height * aspect);
  const h = w / aspect;
  const left = (width - w) / 2;
  const top = (height - h) / 2;

  ctx.fillStyle = css("--sunken");
  ctx.fillRect(left, top, w, h);
  ctx.strokeStyle = css("--line");
  ctx.lineWidth = 1;
  ctx.beginPath();
  for (let i = 1; i < 4; i++) {
    ctx.moveTo(Math.round(left + (w * i) / 4) + 0.5, top);
    ctx.lineTo(Math.round(left + (w * i) / 4) + 0.5, top + h);
    ctx.moveTo(left, Math.round(top + (h * i) / 4) + 0.5);
    ctx.lineTo(left + w, Math.round(top + (h * i) / 4) + 0.5);
  }
  ctx.stroke();

  if (layers.heat && t.peakMovement > 0) {
    const [lr, lg, lb] = css("--heat-lo").split(",").map(Number);
    const [hr, hg, hb] = css("--heat-hi").split(",").map(Number);
    const hctx = heat.getContext("2d")!;
    const image = hctx.createImageData(COLUMNS, ROWS);
    for (let i = 0; i < t.movement.length; i++) {
      if (!t.movement[i]) continue;
      const s = Math.log1p(t.movement[i]) / Math.log1p(t.peakMovement);
      image.data.set([lr + (hr - lr) * s, lg + (hg - lg) * s, lb + (hb - lb) * s, 60 + 190 * s], 4 * i);
    }
    hctx.putImageData(image, 0, 0);
    ctx.imageSmoothingEnabled = true;
    ctx.drawImage(heat, left, top, w, h);
  }

  if (layers.path && t.path.length) {
    ctx.strokeStyle = css("--text");
    ctx.globalAlpha = 0.7;
    ctx.lineWidth = 1.25;
    ctx.beginPath();
    let prev: Trace["path"][number] | undefined;
    for (const p of t.path) {
      const x = left + p.x * w;
      const y = top + p.y * h;
      const joined = prev && prev.segment === p.segment && prev.epoch === p.epoch && prev.width === p.width && prev.height === p.height;
      if (joined) ctx.lineTo(x, y);
      else ctx.moveTo(x, y);
      prev = p;
    }
    ctx.stroke();
    ctx.globalAlpha = 1;
  }

  if (layers.clicks) {
    const colors = buttonColors();
    ctx.strokeStyle = css("--sunken");
    for (const b of BUTTONS) {
      const cells = t.buttons[b];
      ctx.fillStyle = colors[b];
      for (let i = 0; i < cells.length; i++) {
        if (!cells[i]) continue;
        const x = left + ((i % COLUMNS) + 0.5) * (w / COLUMNS);
        const y = top + (Math.floor(i / COLUMNS) + 0.5) * (h / ROWS);
        ctx.beginPath();
        ctx.arc(x, y, Math.min(12, 3 + 2 * Math.log2(cells[i])), 0, 2 * Math.PI);
        ctx.fill();
        ctx.stroke();
      }
    }
  }

  if (!t.moves && !t.presses) waiting(ctx, width, height, "No mouse input yet");
}

/** drawCamera draws the relative path of the latest right-button hold. */
export function drawCamera(canvas: HTMLCanvasElement, t: Trace): void {
  const { ctx, width, height } = surface(canvas);
  if (t.camera.length < 2) {
    waiting(ctx, width, height, "Hold the right button and move to see it");
    return;
  }
  const xs = t.camera.map((p) => p.x);
  const ys = t.camera.map((p) => p.y);
  const minX = Math.min(0, ...xs), maxX = Math.max(0, ...xs);
  const minY = Math.min(0, ...ys), maxY = Math.max(0, ...ys);
  const scale = Math.min((width - 24) / Math.max(1, maxX - minX), (height - 24) / Math.max(1, maxY - minY));
  const x = (raw: number) => width / 2 + (raw - (minX + maxX) / 2) * scale;
  const y = (raw: number) => height / 2 + (raw - (minY + maxY) / 2) * scale;
  ctx.strokeStyle = css("--line-strong");
  ctx.lineWidth = 1;
  ctx.beginPath();
  ctx.moveTo(Math.round(x(0)) + 0.5, 0);
  ctx.lineTo(Math.round(x(0)) + 0.5, height);
  ctx.moveTo(0, Math.round(y(0)) + 0.5);
  ctx.lineTo(width, Math.round(y(0)) + 0.5);
  ctx.stroke();
  ctx.strokeStyle = css("--text-2");
  ctx.lineWidth = 1.5;
  ctx.lineJoin = "round";
  ctx.beginPath();
  t.camera.forEach((p, i) => (i ? ctx.lineTo(x(p.x), y(p.y)) : ctx.moveTo(x(p.x), y(p.y))));
  ctx.stroke();
  ctx.fillStyle = css("--accent");
  ctx.beginPath();
  ctx.arc(x(t.cameraX), y(t.cameraY), 3.5, 0, 2 * Math.PI);
  ctx.fill();
}

/** drawActivity draws moves per second as bars and presses as dots, for the last minute. */
export function drawActivity(canvas: HTMLCanvasElement, t: Trace): void {
  const { ctx, width, height } = surface(canvas);
  const last = Math.floor(t.elapsedUs / 1_000_000);
  const first = Math.max(0, last - RECENT_SECONDS + 1);
  const cell = width / RECENT_SECONDS;
  const visible = t.activity.filter((e) => e.second >= first && e.second <= last);
  if (!visible.length) {
    waiting(ctx, width, height, "No mouse input yet");
    return;
  }
  const peak = Math.max(1, ...visible.map((e) => e.moves));
  const pressPeak = Math.max(1, ...visible.map((e) => e.presses));
  const bottom = height - 20;
  ctx.fillStyle = css("--text-3");
  for (const e of visible) {
    const x = (e.second - first) * cell;
    const bar = (e.moves / peak) * (bottom - 12);
    ctx.fillRect(x, bottom - bar, Math.max(1, cell - 2), bar);
  }
  ctx.fillStyle = css("--accent");
  for (const e of visible) {
    if (!e.presses) continue;
    ctx.beginPath();
    ctx.arc((e.second - first) * cell + cell / 2, bottom - 6 - (e.presses / pressPeak) * (bottom - 20), 3, 0, 2 * Math.PI);
    ctx.fill();
  }
  ctx.fillStyle = css("--text-3");
  ctx.font = `11px ${css("--mono")}`;
  ctx.textBaseline = "alphabetic";
  ctx.textAlign = "left";
  ctx.fillText(formatSeconds(first), 0, height - 4);
  ctx.textAlign = "right";
  ctx.fillText(formatSeconds(first + RECENT_SECONDS - 1), width, height - 4);
}
