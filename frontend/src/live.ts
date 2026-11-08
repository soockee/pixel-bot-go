// Live tab: the capture with the locked bobber, the fishing cycle and the
// area watched for a bite.
import { RefreshCw, X } from "lucide";
import { framesUrl, Service, type State } from "./api";
import { $, clock, h, setIcon } from "./dom";

/** The fishing cycle in order, keyed by fishing.FishingState names. */
const cycle: [state: string, label: string][] = [
  ["focus", "Wait for the game window"],
  ["searching", "Search for the bobber"],
  ["monitoring", "Watch for a bite"],
  ["reeling", "Reel in"],
  ["cooldown", "Cool down"],
  ["casting", "Cast"],
];

/** stepName names a fishing state for the top bar. */
export function stepName(fsm: string): string {
  return { focus: "Focus", searching: "Search", monitoring: "Watch", reeling: "Reel", cooldown: "Cooldown", casting: "Cast" }[fsm] ?? "Stopped";
}

const LOCK_SIZE = 28; // px on screen around the click point

export function setupLive(): (state: State) => void {
  const img = $<HTMLImageElement>("capture-img");
  const frame = $("frame");
  const stage = $("stage");
  const lock = $("lock");
  const empty = $("capture-empty");
  const emptyText = $("capture-empty-text");
  const roi = $<HTMLImageElement>("roi-img");
  const roiEmpty = $("roi-empty");
  const windowSelect = $<HTMLSelectElement>("window");
  const start = $<HTMLButtonElement>("start");
  const selectArea = $<HTMLButtonElement>("select-area");
  const clearArea = $<HTMLButtonElement>("clear-area");
  const area = $("area");
  const timers = $("timers");
  const target = $("target");
  const meta = $("capture-meta");

  setIcon($("refresh"), RefreshCw);
  setIcon(clearArea, X);

  const steps = cycle.map(([key, label]) => {
    const li = h("li");
    li.dataset.state = key;
    li.append(h("span", label), h("span"));
    return li;
  });
  $("cycle").replaceChildren(...steps);

  let state: State | undefined;
  let shownFrame = 0;
  let shownRoi = 0;
  let stepSince = performance.now();
  let lastFsm = "";

  async function loadWindows() {
    const titles = (await Service.Windows()) ?? [];
    windowSelect.replaceChildren(...titles.map((t) => new Option(t, t)));
    const s = await Service.State();
    windowSelect.value = s.window;
  }

  // Fit the frame to the image's aspect inside the stage, so the brackets
  // and the lock box sit on the image itself.
  function fit() {
    if (!img.naturalWidth) return;
    const pad = 24;
    const w = stage.clientWidth - pad * 2;
    const hgt = stage.clientHeight - pad * 2;
    const scale = Math.min(w / img.naturalWidth, hgt / img.naturalHeight);
    frame.style.width = `${Math.max(1, Math.floor(img.naturalWidth * scale))}px`;
    frame.style.height = `${Math.max(1, Math.floor(img.naturalHeight * scale))}px`;
    placeLock();
  }

  function placeLock() {
    if (!state?.locked || !img.naturalWidth || !state.frame) {
      lock.hidden = true;
      return;
    }
    const scale = frame.clientWidth / img.naturalWidth;
    const x = state.target.x * scale - LOCK_SIZE / 2;
    const y = state.target.y * scale - LOCK_SIZE / 2;
    lock.style.width = lock.style.height = `${LOCK_SIZE}px`;
    lock.style.transform = `translate(${x}px, ${y}px)`;
    lock.hidden = false;
  }

  img.addEventListener("load", fit);
  new ResizeObserver(fit).observe(stage);

  windowSelect.addEventListener("change", () => void Service.SetWindow(windowSelect.value));
  $("refresh").addEventListener("click", () => void loadWindows());
  start.addEventListener("click", () => void Service.ToggleCapture());
  selectArea.addEventListener("click", () => void Service.OpenSelection());
  clearArea.addEventListener("click", () => void Service.ClearSelection());

  // The time in the current step ticks between state events.
  setInterval(() => {
    const current = steps.find((li) => li.classList.contains("current"));
    if (current) current.lastElementChild!.textContent = `${((performance.now() - stepSince) / 1000).toFixed(1)} s`;
  }, 100);

  void loadWindows();

  return (s) => {
    state = s;
    const hasArea = s.selection.w > 0;
    const showFrame = s.running && s.frame > 0;

    if (showFrame && s.frame !== shownFrame) img.src = framesUrl("capture", (shownFrame = s.frame));
    frame.hidden = !showFrame;
    empty.hidden = showFrame;
    emptyText.textContent = !hasArea
      ? "Select a capture area over the game first"
      : s.running ? "Waiting for the first frame" : "Start to see the capture area here";
    meta.textContent = hasArea ? `${s.selection.w} × ${s.selection.h} px` : "";
    placeLock();

    const showRoi = s.running && s.roi > 0;
    if (showRoi && s.roi !== shownRoi) roi.src = framesUrl("roi", (shownRoi = s.roi));
    roi.hidden = !showRoi;
    roiEmpty.hidden = showRoi;
    target.textContent = s.locked ? `x ${s.target.x}  y ${s.target.y}` : "";

    if (document.activeElement !== windowSelect && windowSelect.value !== s.window) {
      if (![...windowSelect.options].some((o) => o.value === s.window)) windowSelect.add(new Option(s.window, s.window));
      windowSelect.value = s.window;
    }
    area.textContent = hasArea ? `${s.selection.w} × ${s.selection.h} at ${s.selection.x}, ${s.selection.y}` : "None";
    area.title = area.textContent;
    clearArea.disabled = !hasArea || s.running;
    selectArea.textContent = s.selecting ? "Selecting" : hasArea ? "Change" : "Select";
    selectArea.disabled = s.running;
    start.textContent = s.running ? "Stop" : "Start";
    start.classList.toggle("primary", !s.running);
    start.classList.toggle("stop", s.running);
    start.disabled = s.recording;

    const fsm = s.running ? s.fsm : "";
    if (fsm !== lastFsm) {
      lastFsm = fsm;
      stepSince = performance.now();
    }
    for (const li of steps) {
      const on = li.dataset.state === fsm;
      li.classList.toggle("current", on);
      if (!on) li.lastElementChild!.textContent = "";
    }
    timers.textContent = `${clock(s.sessionSeconds)} session · ${clock(s.totalSeconds)} total`;
  };
}
