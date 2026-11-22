// Fishing view: the live capture with the locked bobber, the cycle, and how
// the run is going against the limits that stop it.
import { framesUrl, Service, type State } from "./api";
import { $, clock, h } from "./dom";
import { caption as captionFor, catches, cycle } from "./cycle";

export function setupLive(): (state: State) => void {
  const img = $<HTMLImageElement>("capture-img");
  const frame = $("frame");
  const stage = $("stage");
  const lock = $("lock");
  const lockImg = $<HTMLImageElement>("lock-img");
  const empty = $("capture-empty");
  const emptyText = $("capture-empty-text");
  const emptyHint = $("capture-empty-hint");
  const caption = $("caption");
  const captionText = $("caption-text");
  const captionTime = $("caption-time");
  const roi = $<HTMLImageElement>("roi-img");
  const roiEmpty = $("roi-empty");
  const windowSelect = $<HTMLSelectElement>("window");
  const start = $<HTMLButtonElement>("start");

  const steps = cycle.map(([key, name]) => {
    const li = h("li", name);
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
  let shownReels = -1;
  // A catch lights the count for a moment.
  const count = $("reels");
  const bump = () => {
    count.classList.remove("bump");
    void count.offsetWidth; // restart the animation
    count.classList.add("bump");
  };
  count.addEventListener("animationend", () => count.classList.remove("bump"));

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

  // warn marks a limit one step away; only a running bot can still reach it.
  function meter(id: string, value: number, limit: number, text: string, warn: boolean) {
    $(`${id}-text`).textContent = text;
    const bar = $(`${id}-bar`);
    bar.style.transform = `scaleX(${limit > 0 ? Math.min(1, value / limit) : 0})`;
    bar.classList.toggle("warn", warn && limit > 1 && value >= limit - 1);
  }

  img.addEventListener("load", fit);
  new ResizeObserver(fit).observe(stage);

  windowSelect.addEventListener("change", () => void Service.SetWindow(windowSelect.value).then(() => Service.CheckGame()));
  windowSelect.addEventListener("mousedown", () => void loadWindows()); // the game may have opened since
  start.addEventListener("click", () => void Service.ToggleCapture());
  // The coffee page opens in the browser and the card hides until the next milestone.
  $("support-open").addEventListener("click", () => void Service.OpenSupport());
  $("support-dismiss").addEventListener("click", () => void Service.DismissSupport());
  // Both mean: never ask again. The status bar link stays.
  for (const id of ["support-already", "support-never"]) $(id).addEventListener("click", () => void Service.MuteSupport());

  // The time in the current step ticks between state events.
  setInterval(() => {
    if (!caption.hidden) captionTime.textContent = `${((performance.now() - stepSince) / 1000).toFixed(1)} s`;
  }, 100);

  void loadWindows();

  return (s) => {
    state = s;
    const showFrame = s.running && s.frame > 0;

    if (showFrame && s.frame !== shownFrame) img.src = framesUrl("capture", (shownFrame = s.frame));
    frame.hidden = !showFrame;
    empty.hidden = showFrame;
    emptyText.textContent = s.running ? "Waiting for the game window" : "Ready to fish";
    emptyHint.textContent = s.running ? "Bring WoW to the front" : "Start fishing, then keep WoW in front. Go Fish shows what it sees here.";
    if (s.roi && s.roi !== shownRoi) roi.src = framesUrl("roi", (shownRoi = s.roi));
    if (s.scene && s.scene !== shownScene) lockImg.src = framesUrl("scene", (shownScene = s.scene));
    fit();

    const fsm = s.running ? s.fsm : "";
    if (fsm !== lastFsm) {
      lastFsm = fsm;
      stepSince = performance.now();
    }
    const step = captionFor(fsm);
    caption.hidden = !step;
    captionText.textContent = step;
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
    start.textContent = s.running ? "Stop" : "Start fishing";
    start.classList.toggle("primary", !s.running && s.setupDone);
    start.classList.toggle("stop", s.running);
    // Starting stays possible, in case a check is wrong, but says what is missing.
    const setup = s.setup.filter((step) => step.id !== "");
    const next = setup.findIndex((step) => !step.done);
    $("start-hint").hidden = s.running || s.setupDone || next < 0;
    $("start-hint").textContent = `Finish setup first: ${setup[next]?.title}, step ${next + 1} of ${setup.length}`;

    if (s.reels > shownReels && shownReels >= 0) bump(); // not for the count the window opens with
    shownReels = s.reels;
    $("reels").textContent = String(s.reels);
    $("reels-label").textContent = catches(s.reels);
    const blocked = s.reelAttempts - s.reelInputs;
    $("casts").textContent = !s.casts ? "No casts yet"
      : `${s.casts} ${s.casts === 1 ? "cast" : "casts"}${blocked > 0 ? ` · ${blocked} ${blocked === 1 ? "reel" : "reels"} blocked` : ""}`;
    // How reels ended, from Bobber Marker: "not hooked" is a reel on a false bite.
    const told = s.caught + s.gotAway + s.notHooked;
    $("outcomes").hidden = told === 0;
    for (const [id, n] of [["caught", s.caught], ["away", s.gotAway], ["missed", s.notHooked]] as const) {
      $(`split-${id}`).style.flexGrow = String(n);
      $(`n-${id}`).textContent = String(n);
    }
    meter("session", s.sessionSeconds, s.sessionLimit * 60, `${clock(s.sessionSeconds)} of ${s.sessionLimit} min`, s.running);
    meter("failed", s.failedInRow, s.maxFailed, `${s.failedInRow} of ${s.maxFailed}`, s.running);

    // The thank-you card: Go decides when it shows (ui/desktop/support.go), at
    // catch milestones after a good run.
    const thanks = s.support;
    $("support-card").hidden = !thanks.ask;
    if (thanks.ask) {
      const hours = Math.floor(thanks.seconds / 3600);
      const fished = hours >= 1 ? `${hours} ${hours === 1 ? "hour" : "hours"}` : `${Math.max(1, Math.round(thanks.seconds / 60))} minutes`;
      $("support-text").textContent = `Go Fish has caught ${thanks.catches.toLocaleString()} fish for you, in ${fished} of fishing you did not have to click through. It is made by one person in their spare time. If that is worth something to you, here is how to say thanks.`;
    }
  };
}
