// Fishing view: the live capture with the locked bobber, the cycle, and how
// the run is going against the limits that stop it.
import { framesUrl, Service, type State } from "./api";
import { $, clock, h } from "./dom";
import { themeById } from "./themes";

/** The fishing cycle in order, as fishing.FishingState names; the theme names each step. */
const cycle = ["focus", "searching", "monitoring", "reeling", "cooldown", "casting"];

export function setupLive(): (state: State) => void {
  const img = $<HTMLImageElement>("capture-img");
  const frame = $("frame");
  const stage = $("stage");
  const lock = $("lock");
  const lockImg = $<HTMLImageElement>("lock-img");
  const empty = $("capture-empty");
  const emptyText = $("capture-empty-text");
  const caption = $("caption");
  const captionText = $("caption-text");
  const captionTime = $("caption-time");
  const roi = $<HTMLImageElement>("roi-img");
  const roiEmpty = $("roi-empty");
  const windowSelect = $<HTMLSelectElement>("window");
  const start = $<HTMLButtonElement>("start");

  const steps = cycle.map((key) => {
    const li = h("li");
    li.dataset.state = key;
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
    const current = windowSelect.value;
    windowSelect.replaceChildren(...titles.map((t) => new Option(t, t)));
    windowSelect.value = (await Service.State()).window || current;
  }

  // Fit the frame to the capture's aspect inside the stage, so the brackets
  // and the lock box sit on the image itself.
  function fit() {
    if (!state?.frameSize.x) return;
    const pad = 24;
    const { x: w, y: hgt } = state.frameSize;
    const scale = Math.min((stage.clientWidth - pad * 2) / w, (stage.clientHeight - pad * 2) / hgt);
    frame.style.width = `${Math.max(1, Math.floor(w * scale))}px`;
    frame.style.height = `${Math.max(1, Math.floor(hgt * scale))}px`;
    placeLock();
  }

  // While a bobber is watched only its box is captured, so the lock shows that
  // box live over the last view of the whole window.
  function placeLock() {
    const box = state?.roiBox;
    if (!state?.frame || !state.roi || !box?.w) {
      lock.hidden = true;
      return;
    }
    const scale = frame.clientWidth / state.frameSize.x;
    lock.style.width = `${box.w * scale}px`;
    lock.style.height = `${box.h * scale}px`;
    lock.style.transform = `translate(${box.x * scale}px, ${box.y * scale}px)`;
    lock.hidden = false;
  }

  function meter(id: string, value: number, limit: number, text: string) {
    $(`${id}-text`).textContent = text;
    const bar = $(`${id}-bar`);
    bar.style.transform = `scaleX(${limit > 0 ? Math.min(1, value / limit) : 0})`;
    bar.classList.toggle("warn", limit > 1 && value >= limit - 1);
  }

  img.addEventListener("load", fit);
  new ResizeObserver(fit).observe(stage);

  windowSelect.addEventListener("change", () => void Service.SetWindow(windowSelect.value));
  windowSelect.addEventListener("mousedown", () => void loadWindows()); // the game may have opened since
  start.addEventListener("click", () => void Service.ToggleCapture());

  // The time in the current step ticks between state events.
  setInterval(() => {
    if (!caption.hidden) captionTime.textContent = `${((performance.now() - stepSince) / 1000).toFixed(1)} s`;
  }, 100);

  void loadWindows();

  return (s) => {
    state = s;
    const words = themeById(s.theme).words;
    steps.forEach((li, i) => (li.textContent = words.cycle[cycle[i]][0]));
    const showFrame = s.running && s.frame > 0;

    if (showFrame && s.frame !== shownFrame) img.src = framesUrl("capture", (shownFrame = s.frame));
    frame.hidden = !showFrame;
    empty.hidden = showFrame;
    emptyText.textContent = s.running ? "Waiting for the game window" : words.empty;
    if (s.roi && s.roi !== shownRoi) {
      shownRoi = s.roi;
      lockImg.src = roi.src = framesUrl("roi", s.roi);
    }
    fit();

    const fsm = s.running ? s.fsm : "";
    if (fsm !== lastFsm) {
      lastFsm = fsm;
      stepSince = performance.now();
    }
    const step = words.cycle[fsm];
    caption.hidden = !step;
    if (step) captionText.textContent = step[1];
    for (const li of steps) li.classList.toggle("current", li.dataset.state === fsm);

    $("watch-panel").hidden = !s.debug;
    if (s.debug) {
      const showRoi = s.running && s.roi > 0;
      roi.hidden = !showRoi;
      roiEmpty.hidden = showRoi;
      $("target").textContent = showRoi ? `${s.roiBox.w} × ${s.roiBox.h} at ${s.roiBox.x}, ${s.roiBox.y}` : "";
    }

    if (document.activeElement !== windowSelect && windowSelect.value !== s.window) {
      if (![...windowSelect.options].some((o) => o.value === s.window)) windowSelect.add(new Option(s.window, s.window));
      windowSelect.value = s.window;
    }
    windowSelect.disabled = s.running;
    start.textContent = s.running ? words.stop : words.start;
    start.classList.toggle("primary", !s.running);
    start.classList.toggle("stop", s.running);
    // Fishing needs a role that may fish; a dry run sends no input and needs none.
    const allowed = s.running || s.dryRun || s.canFish;
    start.disabled = s.recording || !allowed;
    $("start-hint").hidden = allowed;
    $("start-hint").textContent = s.account
      ? "Your role may not fish. Ask the owner to give your account a role."
      : "Sign in with Google to fish";

    $("reels").textContent = String(s.reels);
    $("reels-label").textContent = words.catches[s.reels === 1 ? 0 : 1];
    $("casts").textContent = s.casts ? `${s.casts} casts · ${s.reelInputs} of ${s.reelAttempts} reel inputs sent` : "No casts yet";
    meter("session", s.sessionSeconds, s.sessionLimit * 60, `${clock(s.sessionSeconds)} of ${s.sessionLimit} min`);
    meter("failed", s.failedInRow, s.maxFailed, `${s.failedInRow} of ${s.maxFailed}`);
  };
}
