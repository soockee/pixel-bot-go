// Recorder tab: start and stop recordings, and review one recording's input,
// live while it records.
import { RefreshCw } from "lucide";
import { recordingFrameUrl, Service, type Recording, type State } from "./api";
import { drawActivity, drawCamera, drawMap, buttonColors } from "./charts";
import { $, css, h, setIcon } from "./dom";
import { formatSeconds, Trace } from "./trace";

const POLL_MS = 400;

/** recordingLabel turns a 20261108-012132 directory name into a date and time. */
function recordingLabel(name: string): string {
  const m = /^(\d{4})(\d{2})(\d{2})-(\d{2})(\d{2})(\d{2})$/.exec(name);
  if (!m) return name;
  const d = new Date(+m[1], +m[2] - 1, +m[3], +m[4], +m[5], +m[6]);
  return `${d.toLocaleDateString(undefined, { day: "numeric", month: "short" })}, ${d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit", second: "2-digit" })}`;
}

export function setupRecorder(isVisible: () => boolean): { onState: (s: State) => void; shown: () => void } {
  const record = $<HTMLButtonElement>("record");
  const list = $("recordings");
  const title = $("trace-title");
  const status = $("trace-status");
  const stats = $("stats");
  const map = $<HTMLCanvasElement>("map");
  const camera = $<HTMLCanvasElement>("camera");
  const activity = $<HTMLCanvasElement>("activity");
  const frameImg = $<HTMLImageElement>("trace-frame");
  const layers = { heat: $<HTMLInputElement>("layer-heat"), clicks: $<HTMLInputElement>("layer-clicks"), path: $<HTMLInputElement>("layer-path") };
  const manualOnly = [$("map-panel"), $("camera-panel"), $("activity-panel")];

  setIcon($("refresh-recordings"), RefreshCw);

  let recordings: Recording[] = [];
  let open = ""; // the recording shown
  let live = ""; // the recording being written, or ""
  let trace = new Trace();
  let offset = 0;
  let loading = 0; // generation, so a stale load stops
  let error = "";
  let queued = false;
  let state: State | undefined;
  const upload = $<HTMLButtonElement>("upload");
  const reviewDialog = $<HTMLDialogElement>("upload-review");
  const reviewSend = $<HTMLButtonElement>("upload-review-send");
  const reviewFrames = $<HTMLInputElement>("upload-review-frames");
  const reviewAudio = $<HTMLInputElement>("upload-review-audio");
  const reviewError = $("upload-review-error");
  const reviewImage = $<HTMLImageElement>("upload-review-image");
  const reviewSlider = $<HTMLInputElement>("upload-review-slider");
  const reviewPlayer = $<HTMLAudioElement>("upload-review-player");
  let reviewFrameNames: string[] = [];

  function previewReviewFrame() {
    if (!reviewed) return;
    const frame = reviewFrameNames[Number(reviewSlider.value)];
    if (!frame) return;
    reviewImage.src = `/recordings/${encodeURIComponent(reviewed.name)}?frame=${encodeURIComponent(frame)}`;
    $("upload-review-frame-time").textContent = `${(Number.parseInt(frame, 10) / 1000).toFixed(1)} s`;
  }
  let reviewed: { name: string; fingerprint: string; hasInput: boolean } | undefined;

  function renderReviewSend() {
    reviewSend.disabled = !reviewed || !(reviewed.hasInput || reviewFrames.checked || reviewAudio.checked);
  }

  async function reviewUpload() {
    const name = open;
    reviewed = undefined;
    reviewError.textContent = "";
    $("upload-review-preview").hidden = true;
    reviewPlayer.hidden = true;
    $("upload-review-summary").textContent = "Checking the local recording…";
    $("upload-review-warning").textContent = "";
    $("upload-review-sensitive").replaceChildren();
    reviewFrames.disabled = reviewAudio.disabled = true;
    reviewFrames.checked = reviewAudio.checked = false;
    renderReviewSend();
    reviewDialog.showModal();
    try {
      const review = await Service.ReviewUpload(name);
      if (!reviewDialog.open || name !== open) return;
      reviewed = { name, fingerprint: review.fingerprint, hasInput: review.hasInput };
      $("upload-review-summary").textContent = `${review.frames} screen frames available. ${review.hasInput ? "Pointer activity will be included." : "No input trace is available."} ${review.keyEvents} key events will be excluded.`;
      $("upload-review-warning").textContent = review.sensitiveCount
        ? `${review.sensitiveCount} typing bursts may contain passwords or chat. Their contents are hidden. All recorded keys will be discarded from the upload copy.`
        : "All recorded keys are discarded from the upload copy. Check frames and audio for personal information before including them.";
      $("upload-review-sensitive").replaceChildren(...(review.sensitive ?? []).map(span =>
        h("li", `${(span.startUs / 1e6).toFixed(1)}–${(span.endUs / 1e6).toFixed(1)} s · ${span.presses} presses · excluded`, "sensitive-span")
      ));
      reviewFrameNames = review.frameNames ?? [];
      reviewFrames.disabled = reviewFrameNames.length === 0;
      $("upload-review-preview").hidden = reviewFrames.disabled;
      reviewSlider.max = String(Math.max(0, reviewFrameNames.length - 1));
      reviewSlider.value = "0";
      previewReviewFrame();
      reviewAudio.disabled = !review.hasAudio;
      reviewPlayer.hidden = !review.hasAudio;
      if (review.hasAudio) reviewPlayer.src = `/recordings/${encodeURIComponent(name)}?audio=1`;
      renderReviewSend();
    } catch (e) {
      if (reviewDialog.open) reviewError.textContent = String(e);
    }
  }

  // Keep upload out of the way unless this account may upload recordings.
  function renderUpload() {
    upload.hidden = !open || open === live || !state || !state.canUpload;
    if (!state) return;
    upload.disabled = !state.canUpload || state.uploading !== "";
    upload.textContent = state.uploading === open ? "Uploading…" : "Review upload";
    upload.title = "Review this recording before uploading";
  }

  async function refreshList() {
    try {
      recordings = (await Service.Recordings()) ?? [];
    } catch (e) {
      recordings = [];
      error = String(e);
    }
    renderList();
  }

  function renderList() {
    const items = recordings.map((r) => {
      const b = h("button");
      b.type = "button";
      b.setAttribute("aria-current", String(r.name === open));
      const tag = r.name === live ? h("span", "Recording", "tag live") : h("span", r.hasInput ? "" : "Frames only", "tag");
      b.append(h("span", recordingLabel(r.name)), tag);
      b.addEventListener("click", () => select(r.name));
      return h("li").appendChild(b).parentElement!;
    });
    list.replaceChildren(...(items.length ? items : [h("li", "No recordings yet", "none")]));
  }

  async function select(name: string) {
    open = name;
    renderUpload();
    trace = new Trace();
    offset = 0;
    error = "";
    renderList();
    render();
    const generation = ++loading;
    for (;;) {
      try {
        const page = await Service.RecordingEvents(name, offset);
        if (generation !== loading) return;
        for (const e of page.events ?? []) trace.ingest(e);
        offset = page.nextOffset;
        error = "";
        if (page.events?.length) request();
        if (page.more) continue;
      } catch (e) {
        if (generation !== loading) return;
        error = String(e).includes("cannot find") ? "No input trace in this recording" : String(e);
        request();
      }
      frameImg.src = recordingFrameUrl(name);
      if (name !== live || trace.ended) return;
      await new Promise((r) => setTimeout(r, POLL_MS));
      if (generation !== loading) return;
    }
  }

  function request() {
    if (queued) return;
    queued = true;
    requestAnimationFrame(() => {
      queued = false;
      render();
    });
  }

  function stat(label: string, value: string) {
    const div = h("div");
    div.append(h("dt", label), h("dd", value));
    return div;
  }

  function render() {
    title.textContent = open ? recordingLabel(open) : "No recording open";
    status.textContent = !open
      ? "Record a session or pick one from the list"
      : error || (open === live ? (trace.active ? "Recording, following live" : "Recording, waiting for the target window to have focus")
        : trace.ended ? `Saved, ${trace.ended}` : "Saved");
    const n = (v: number) => (trace.schema === 1 ? "–" : v.toLocaleString());
    $("trace-body").hidden = !open;
    stats.replaceChildren(...(!open ? [] : trace.manual
      ? [
        stat("Duration", formatSeconds(Math.floor(trace.elapsedUs / 1_000_000))),
        stat("Moves", trace.moves.toLocaleString()),
        stat("Presses", trace.presses.toLocaleString()),
        stat("Clicks", n(trace.clicks)),
        stat("Drags", n(trace.drags)),
        stat("Camera drags", n(trace.rightDrags)),
        stat("Keys", n(trace.keys)),
      ]
      : [
        stat("Casts", trace.semanticCounts.casts.toLocaleString()),
        stat("Bites available", trace.semanticCounts.availableBites.toLocaleString()),
        stat("Bites detected", trace.semanticCounts.detectedBites.toLocaleString()),
        stat("Reels", trace.semanticCounts.reels.toLocaleString()),
      ]));
    for (const panel of manualOnly) panel.hidden = !trace.manual;
    $("frame-panel").hidden = !open;

    const colors = buttonColors();
    const heatKey = h("i", "", "heat");
    heatKey.style.background = `linear-gradient(90deg, rgb(${css("--heat-lo")}), rgb(${css("--heat-hi")}))`;
    const legend = [h("span")];
    legend[0].append(heatKey, "Movement");
    for (const [b, label] of [["left", "Left click"], ["right", "Right click"], ["middle", "Middle"], ["x1", "Side buttons"]] as const) {
      const s = h("span");
      const dot = h("i");
      dot.style.background = colors[b];
      s.append(dot, label);
      legend.push(s);
    }
    $("legend").replaceChildren(...legend);
    if (trace.clientWidth) $("legend").append(h("span", `Client area ${trace.clientWidth} × ${trace.clientHeight} px`));

    if (isVisible() && trace.manual) {
      drawMap(map, trace, { heat: layers.heat.checked, clicks: layers.clicks.checked, path: layers.path.checked });
      drawCamera(camera, trace);
      drawActivity(activity, trace);
    }
    $("input-log").replaceChildren(...trace.log.slice().reverse().map((row) => {
      const tr = h("tr");
      tr.append(h("td", row.time), h("td", row.input), h("td", row.detail));
      return tr;
    }));
  }

  record.addEventListener("click", () => void Service.ToggleRecord());
  upload.addEventListener("click", () => void reviewUpload());
  reviewSlider.addEventListener("input", previewReviewFrame);
  reviewDialog.addEventListener("close", () => {
    reviewPlayer.pause();
    reviewPlayer.removeAttribute("src");
    reviewPlayer.load();
    reviewImage.removeAttribute("src");
  });
  reviewFrames.addEventListener("change", renderReviewSend);
  reviewAudio.addEventListener("change", renderReviewSend);
  reviewSend.addEventListener("click", () => {
    const approved = reviewed;
    if (!approved) return;
    const frames = reviewFrames.checked;
    const audio = reviewAudio.checked;
    reviewed = undefined;
    reviewDialog.close();
    void Service.Upload(approved.name, approved.fingerprint, frames, audio).catch(e => {
      error = String(e);
      request();
    });
  });
  $("refresh-recordings").addEventListener("click", () => void refreshList());
  for (const input of Object.values(layers)) input.addEventListener("change", request);
  const resize = new ResizeObserver(request);
  for (const c of [map, camera, activity]) resize.observe(c);
  frameImg.addEventListener("error", () => ($("frame-panel").hidden = true));
  frameImg.addEventListener("load", () => ($("frame-panel").hidden = false));

  void refreshList();
  render();

  return {
    onState(s) {
      state = s;
      record.textContent = s.recording ? "Stop recording" : "Record";
      record.classList.toggle("primary", !s.recording);
      record.classList.toggle("stop", s.recording);
      record.disabled = s.running;
      $("record-hint").textContent = s.running
        ? "Stop the run to record"
        : "Records the game window, plus your mouse, keys and sound in the target window";
      if (s.recordingName !== live) {
        const started = s.recordingName;
        live = started;
        void refreshList().then(() => {
          if (started) void select(started); // follow a new recording
          else if (open) void select(open); // reload the finished one with its footer
        });
      }
      renderUpload();
    },
    shown: request,
  };
}
