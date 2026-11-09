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
  let shownScene = 0;
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

  // While a bobber is watched only the area around it is captured, so the lock
  // shows that area live over the last view of the whole window, and the frame
  // zooms in on it. Leaving the watch zooms back out.
  function placeLock() {
    const box = state?.sceneBox;
    if (!state?.frame || !state.scene || !box?.w) {
      lock.hidden = true;
      frame.classList.remove("zoomed");
      frame.style.transform = "";
      return;
    }
    const scale = frame.clientWidth / state.frameSize.x;
    const [x, y, w, hgt] = [box.x * scale, box.y * scale, box.w * scale, box.h * scale];
    lock.style.width = `${w}px`;
    lock.style.height = `${hgt}px`;
    lock.style.transform = `translate(${x}px, ${y}px)`;
    lock.hidden = false;
    // Scale about the frame's corner, then move the area's centre to the
    // frame's centre, which the stage centres.
    const zoom = Math.min((stage.clientWidth * 0.9) / w, (stage.clientHeight * 0.9) / hgt, 8);
    const tx = frame.clientWidth / 2 - zoom * (x + w / 2);
    const ty = frame.clientHeight / 2 - zoom * (y + hgt / 2);
    frame.classList.add("zoomed");
    frame.style.transform = `translate(${tx}px, ${ty}px) scale(${zoom})`;
  }

  function meter(id: string, value: number, limit: number, text: string) {
    $(`${id}-text`).textContent = text;
    const bar = $(`${id}-bar`);
    bar.style.transform = `scaleX(${limit > 0 ? Math.min(1, value / limit) : 0})`;
    bar.classList.toggle("warn", limit > 1 && value >= limit - 1);
  }

  img.addEventListener("load", fit);
  new ResizeObserver(fit).observe(stage);

  windowSelect.addEventListener("change", () => void Service.SetWindow(windowSelect.value).then(() => Service.CheckGame()));
  windowSelect.addEventListener("mousedown", () => void loadWindows()); // the game may have opened since
  start.addEventListener("click", () => void Service.ToggleCapture());
  // The coffee page opens in the browser; Go remembers it and hides the card.
  $("support-open").addEventListener("click", () => void Service.OpenSupport());
  $("support-dismiss").addEventListener("click", () => void Service.DismissSupport());

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
    if (s.roi && s.roi !== shownRoi) roi.src = framesUrl("roi", (shownRoi = s.roi));
    if (s.scene && s.scene !== shownScene) lockImg.src = framesUrl("scene", (shownScene = s.scene));
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
    start.classList.toggle("primary", !s.running && s.setupDone);
    start.classList.toggle("stop", s.running);
    start.disabled = s.recording;
    $("start-hint").hidden = true;

    $("reels").textContent = String(s.reels);
    $("reels-label").textContent = words.catches[s.reels === 1 ? 0 : 1];
    $("casts").textContent = s.casts ? `${s.casts} casts · ${s.reelInputs} of ${s.reelAttempts} reel inputs sent` : "No casts yet";
    meter("session", s.sessionSeconds, s.sessionLimit * 60, `${clock(s.sessionSeconds)} of ${s.sessionLimit} min`);
    meter("failed", s.failedInRow, s.maxFailed, `${s.failedInRow} of ${s.maxFailed}`);

    // The thank-you card: Go decides when it shows (ui/desktop/support.go).
    const thanks = s.support;
    $("support-card").hidden = !thanks.ask;
    if (thanks.ask) {
      $("support-text").textContent = `Go Fish landed ${thanks.runCatches} fish this run and ${thanks.catches.toLocaleString()} for you so far. It is made by one person in their spare time. If it saves you time, a coffee helps keep it going.`;
    }
  };
}
