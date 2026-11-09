// Main window: tabs and the status bar. Fishing is the app; the Recorder tab
// is shown when the person shares recordings or in debug mode; the detection
// settings are developer tools, shown only in debug mode.
import { onState, Service, type State } from "./api";
import { $ } from "./dom";
import { setupLive } from "./live";
import { setupMoveOverlay } from "./move-overlay";
import { setupRecorder } from "./recorder";
import { setupSettings } from "./settings";
import { burst, playSound, themeById } from "./themes";
import { setupWizard } from "./setup";
import { setupUpdates } from "./updates";

type Tab = "live" | "recorder" | "settings";
const tabs: Tab[] = ["live", "recorder", "settings"];
let wanted: Tab = "live";
let current: Tab = "live";
let recorderShown = false;

const recorder = setupRecorder(() => current === "recorder");
const visible = () => tabs.filter((t) => t !== "recorder" || recorderShown);

// show shows tab, or Fishing while tab is hidden; the choice is kept for later.
function show(tab: Tab) {
  wanted = tab;
  if (!visible().includes(tab)) tab = "live";
  current = tab;
  for (const t of tabs) {
    $(`tab-${t}`).hidden = !visible().includes(t);
    $(`tab-${t}`).setAttribute("aria-selected", String(t === tab));
    $(`tab-${t}`).tabIndex = t === tab ? 0 : -1;
    $(`view-${t}`).hidden = t !== tab;
  }
  if (tab === "recorder") recorder.shown();
  if (tab === "settings") void Service.CheckAddon(); // the game may have started since
  settings.mountCastKey(tab === "live" ? wizard.keysSlot() : undefined);
}

for (const t of tabs) $(`tab-${t}`).addEventListener("click", () => show(t));
document.querySelector(".tabs")!.addEventListener("keydown", (e) => {
  const key = (e as KeyboardEvent).key;
  if (key !== "ArrowLeft" && key !== "ArrowRight") return;
  const list = visible();
  const next = list[(list.indexOf(current) + (key === "ArrowRight" ? 1 : list.length - 1)) % list.length];
  show(next);
  $(`tab-${next}`).focus();
});

// Go shows the download's progress and any error in the status bar.
$("update").addEventListener("click", () => void Service.Update().catch(() => undefined));
// Opens the coffee page in the browser.
$("support-link").addEventListener("click", () => void Service.OpenSupport());
// Opens a filled-in GitHub issue in the browser; the person submits it there.
$("report-problem").addEventListener("click", () => void Service.ReportProblem());

const live = setupLive();
const moveOverlay = setupMoveOverlay();
const settings = setupSettings();
const wizard = setupWizard(settings.finishSaving);
const updates = setupUpdates();
show(wanted);
let theme = "";
let reels = -1; // -1 until the first state, so opening the window celebrates nothing

function render(s: State) {
  if (s.theme !== theme) {
    theme = s.theme;
    document.documentElement.dataset.theme = theme;
    $<HTMLImageElement>("brand-icon").src = `/themes/${theme}.svg`;
    const words = themeById(theme).words;
    for (const t of tabs) $(`tab-${t}`).textContent = words.tabs[t];
    recorder.shown(); // canvases read the theme colours
  }
  // Themes react to the fishing step, for example a glow while watching.
  document.body.dataset.fsm = s.running ? s.fsm : "";
  if (reels >= 0 && s.reels > reels) {
    playSound(themeById(theme));
    burst(themeById(theme), $("stage"));
  }
  reels = s.reels;
  if (s.debug !== recorderShown) {
    recorderShown = s.debug;
    show(wanted);
  }
  live(s);
  wizard.render(s);
  settings.mountCastKey(current === "live" ? wizard.keysSlot() : undefined);
  moveOverlay(s);
  if (recorderShown) recorder.onState(s);
  settings.render(s);
  updates(s);

  const update = $<HTMLButtonElement>("update");
  update.hidden = s.update === "";
  update.disabled = s.updating || s.running || s.recording;
  update.textContent = s.updating ? `Updating to ${s.update}…` : `Update to ${s.update}`;

  $("status").textContent = s.status;
  $("version").textContent = s.version;
  $("dry-run").hidden = !s.dryRun;
}

// Resolve the game and addon before drawing the first checklist.
void Service.CheckGame().catch(() => undefined).then(() => onState(render));
