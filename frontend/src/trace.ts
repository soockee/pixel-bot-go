// Trace folds the records of one input trace (internal/playtrace) into the
// counts and grids the Recorder tab draws. It has no DOM dependency.

export const COLUMNS = 80;
export const ROWS = 50;
export const PATH_LIMIT = 600;
export const RECENT_SECONDS = 60;
export const BUTTONS = ["left", "right", "middle", "x1", "x2"] as const;
export type Button = (typeof BUTTONS)[number];

const buttonNames: Record<string, string> = { left: "Left", right: "Right", middle: "Middle", x1: "Mouse 4", x2: "Mouse 5" };

/** Semantic records written by dry runs and the fake game, not by a person. */
const semantic: Record<string, [metric: string, label: string]> = {
  cast_dispatched: ["casts", "Cast dispatched"],
  bite_available: ["availableBites", "Bite available"],
  bite_detected: ["detectedBites", "Bite detected"],
  reel_attempt: ["reels", "Simulated reel attempt"],
  reel_dispatched: ["reels", "Dry-run reel dispatched"],
};

export interface PathPoint { x: number; y: number; segment: number; epoch: number; width: number; height: number }
export interface LogRow { time: string; input: string; detail: string }

/* Records are JSON from Go with many optional fields. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Record_ = any;

export class Trace {
  schema = 1;
  source = "manual";
  elapsedUs = 0;
  moves = 0;
  presses = 0;
  segments = 0;
  clicks = 0;
  drags = 0;
  rightDrags = 0;
  keys = 0;
  semanticCounts: Record<string, number> = { casts: 0, availableBites: 0, detectedBites: 0, reels: 0 };
  active = false;
  ended = ""; // "complete" or "incomplete" once the footer arrives
  clientWidth = 0;
  clientHeight = 0;

  movement = new Uint32Array(COLUMNS * ROWS);
  peakMovement = 0;
  buttons = Object.fromEntries(BUTTONS.map((b) => [b, new Uint32Array(COLUMNS * ROWS)])) as Record<Button, Uint32Array>;
  path: PathPoint[] = [];
  activity = Array.from({ length: RECENT_SECONDS }, () => ({ second: -1, moves: 0, presses: 0 }));
  log: LogRow[] = [];
  camera: { x: number; y: number }[] = [];
  cameraX = 0;
  cameraY = 0;

  private pathEpoch = 0;
  private cameraActive = false;

  get manual(): boolean { return this.source === "manual"; }

  ingest(e: Record_): void {
    if (Number.isFinite(e.t_us)) this.elapsedUs = Math.max(this.elapsedUs, e.t_us);
    switch (e.type) {
      case "session_start":
        this.schema = e.schema || 1;
        this.source = e.source || "manual";
        return;
      case "session_end":
        this.ended = e.status === "complete" ? "complete" : "incomplete";
        this.active = false;
        return;
      case "segment_start":
        this.segments++;
        this.active = true;
        return;
      case "segment_end":
        this.active = false;
        this.trackCamera(e);
        return;
    }
    if (!this.manual && e.type in semantic) {
      const [metric, label] = semantic[e.type];
      this.semanticCounts[metric]++;
      const detail = [e.run_id, e.event_id == null ? "" : `event ${e.event_id}`, e.outcome].filter(Boolean);
      this.addLog(e, label, detail.join(" · "));
      return;
    }
    this.trackCamera(e);
    if (e.type === "mouse_gap") {
      this.pathEpoch++;
      return;
    }
    if (e.type === "key_down" && e.key && !e.key.repeat) {
      this.keys++;
      this.addLog(e, e.key.chord || e.key.name, "Key press");
      return;
    }
    if (e.type === "drag" || e.type === "click" || e.type === "gesture_interrupted") {
      if (e.type === "drag") {
        this.drags++;
        if (e.button === "right") this.rightDrags++;
      } else if (e.type === "click") {
        this.clicks++;
      }
      const kind = { drag: "drag", click: "click", gesture_interrupted: "interrupted hold" }[e.type as string];
      const ms = Math.round((e.gesture?.duration_us || 0) / 1000);
      const travel = Math.round(e.gesture?.raw_travel || 0);
      this.addLog(e, `${buttonNames[e.button] || e.button} ${kind}`, `${ms} ms · ${travel} raw counts`);
    }
    if (e.type === "wheel" || e.type === "wheel_horizontal") {
      this.addLog(e, e.type === "wheel" ? "Wheel" : "Horizontal wheel", String(e.wheel));
      return;
    }
    this.place(e);
  }

  /** place adds a positioned move or press to the grids. */
  private place(e: Record_): void {
    const p = e.position;
    if (!p || !Number.isFinite(e.t_us) || e.t_us < 0 ||
        ![p.x, p.y, p.width, p.height].every(Number.isFinite) ||
        p.width <= 0 || p.height <= 0 || p.x < 0 || p.y < 0 || p.x >= p.width || p.y >= p.height) return;
    const mappedClick = e.type === "click" || (this.schema === 1 && e.type === "button_down");
    if (e.type !== "move" && e.type !== "button_down" && !mappedClick) return;

    const x = p.x / p.width;
    const y = p.y / p.height;
    const cell = Math.floor(y * ROWS) * COLUMNS + Math.floor(x * COLUMNS);
    const second = Math.floor(e.t_us / 1_000_000);
    const slot = this.activity[second % RECENT_SECONDS];
    if (slot.second !== second) Object.assign(slot, { second, moves: 0, presses: 0 });
    this.clientWidth = p.width;
    this.clientHeight = p.height;
    if (e.type === "move") {
      this.peakMovement = Math.max(this.peakMovement, ++this.movement[cell]);
      this.path.push({ x, y, segment: e.segment, epoch: this.pathEpoch, width: p.width, height: p.height });
      if (this.path.length > PATH_LIMIT) this.path.shift();
      this.moves++;
      slot.moves++;
      return;
    }
    if (e.type === "button_down") {
      this.presses++;
      slot.presses++;
    }
    if (mappedClick && e.button in this.buttons) this.buttons[e.button as Button][cell]++;
  }

  /** trackCamera follows relative movement while the right button is held. */
  private trackCamera(e: Record_): void {
    if (e.type === "button_down" && e.button === "right") {
      this.cameraActive = true;
      this.cameraX = this.cameraY = 0;
      this.camera = [{ x: 0, y: 0 }];
    } else if ((e.type === "button_up" && e.button === "right") || e.type === "mouse_gap" || e.type === "segment_end") {
      this.cameraActive = false;
    } else if (this.cameraActive && e.type === "move" && (Number.isFinite(e.dx) || Number.isFinite(e.dy))) {
      this.cameraX += e.dx || 0;
      this.cameraY += e.dy || 0;
      this.camera.push({ x: this.cameraX, y: this.cameraY });
      if (this.camera.length > PATH_LIMIT) this.camera.shift();
    }
  }

  private addLog(e: Record_, input: string, detail: string): void {
    const ms = Math.floor((e.t_us || 0) / 1000);
    this.log.push({ time: `${formatSeconds(Math.floor(ms / 1000))}.${String(ms % 1000).padStart(3, "0")}`, input, detail });
    if (this.log.length > 24) this.log.shift();
  }
}

/** formatSeconds formats whole seconds as m:ss. */
export function formatSeconds(seconds: number): string {
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`;
}
