// Main window: tabs and the status bar. Fishing is the app; the Recorder tab
// is shown when the person shares recordings or in debug mode; the detection
// settings are developer tools, shown only in debug mode.
import { onState, Service, type State } from "./api";
import { $ } from "./dom";
import { setupLive } from "./live";
import { setupMoveOverlay } from "./move-overlay";
import { setupRecorder } from "./recorder";
import { setupSettings } from "./settings";
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
  settings.mountSetup(tab === "live" ? wizard.slot() : undefined, wizard.fields());
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
for (const id of ["support-link", "support-updates"]) $(id).addEventListener("click", () => void Service.OpenSupport());
// Opens a filled-in GitHub issue in the browser; the person submits it there.
$("report-problem").addEventListener("click", () => void Service.ReportProblem());
// Go writes the file and shows it in File Explorer; this only shows where, or what failed.
const saveReport = $<HTMLButtonElement>("save-report");
saveReport.addEventListener("click", async () => {
  const detail = $("report-detail");
  saveReport.disabled = true;
  detail.classList.remove("bad");
  try {
    detail.textContent = `Saved ${await Service.SaveReport()}`;
  } catch (e) {
    detail.classList.add("bad");
    detail.textContent = String(e).replace(/^Error: /, "");
  } finally {
    saveReport.disabled = false;
  }
});

const live = setupLive();
const moveOverlay = setupMoveOverlay();
const settings = setupSettings();
const wizard = setupWizard(settings.finishSaving, () => settings.setValue("esp32Port", "AUTO"));
const updates = setupUpdates();
show(wanted);

function render(s: State) {
  if (s.debug !== recorderShown) {
    recorderShown = s.debug;
    show(wanted);
  }
  live(s);
  wizard.render(s);
  settings.mountSetup(current === "live" ? wizard.slot() : undefined, wizard.fields());
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
