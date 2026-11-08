// Main window: tabs, the top bar and the status bar. Each tab module owns its
// panel and gets every state change.
import { Moon, Sun } from "lucide";
import { onState, Service, type State } from "./api";
import { $, setIcon, storage } from "./dom";
import { setupLive, stepName } from "./live";
import { setupRecorder } from "./recorder";
import { setupSettings } from "./settings";

type Tab = "live" | "recorder" | "settings";
const tabs: Tab[] = ["live", "recorder", "settings"];
let current: Tab = (storage.get("tab") as Tab) ?? "live";
if (!tabs.includes(current)) current = "live";

const recorder = setupRecorder(() => current === "recorder");

function show(tab: Tab) {
  current = tab;
  storage.set("tab", tab);
  for (const t of tabs) {
    $(`tab-${t}`).setAttribute("aria-selected", String(t === tab));
    $(`tab-${t}`).tabIndex = t === tab ? 0 : -1;
    $(`view-${t}`).hidden = t !== tab;
  }
  if (tab === "recorder") recorder.shown();
}

for (const t of tabs) $(`tab-${t}`).addEventListener("click", () => show(t));
document.querySelector(".tabs")!.addEventListener("keydown", (e) => {
  const key = (e as KeyboardEvent).key;
  if (key !== "ArrowLeft" && key !== "ArrowRight") return;
  const next = tabs[(tabs.indexOf(current) + (key === "ArrowRight" ? 1 : tabs.length - 1)) % tabs.length];
  show(next);
  $(`tab-${next}`).focus();
});
show(current);

let dark = true;
function applyTheme(isDark: boolean) {
  dark = isDark;
  document.documentElement.dataset.theme = dark ? "dark" : "light";
  setIcon($("theme"), dark ? Sun : Moon);
  $("theme").setAttribute("aria-label", dark ? "Switch to light theme" : "Switch to dark theme");
  recorder.shown(); // canvases read the theme colours
}
$("theme").addEventListener("click", () => {
  applyTheme(!dark);
  void Service.SetDark(dark);
});

const live = setupLive();
const settings = setupSettings();
let themed = false;

onState((s: State) => {
  if (!themed) {
    themed = true;
    applyTheme(s.dark);
  }
  live(s);
  recorder.onState(s);
  settings(s);

  const runState = $("run-state");
  runState.classList.toggle("on", s.running);
  runState.classList.toggle("recording", s.recording);
  $("run-label").textContent = s.recording ? "Recording" : s.running ? stepName(s.fsm) : "Stopped";
  $("status").textContent = s.status;
  $("dry-run").hidden = !s.dryRun;
  $("stop-key").textContent = s.stopKey;
  $("stop-hint").hidden = !s.stopKey;
});
