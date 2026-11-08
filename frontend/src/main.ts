// Main window: tabs and the status bar. Fishing is the app; the Recorder tab
// is shown when the person shares recordings or in debug mode; the detection
// settings are developer tools, shown only in debug mode.
import { onState, Service, type State } from "./api";
import { $, keyLabel, storage } from "./dom";
import { setupLive } from "./live";
import { setupMoveOverlay } from "./move-overlay";
import { setupRecorder } from "./recorder";
import { setupSettings } from "./settings";
import { setupWizard } from "./setup";
import { setupUpdates } from "./updates";

type Tab = "live" | "recorder" | "settings";
const tabs: Tab[] = ["live", "recorder", "settings"];
let wanted: Tab = tabs.includes(storage.get("tab") as Tab) ? (storage.get("tab") as Tab) : "live";
let current: Tab = "live";
let recorderShown = false;

const recorder = setupRecorder(() => current === "recorder");
const visible = () => tabs.filter((t) => t !== "recorder" || recorderShown);

// show shows tab, or Fishing while tab is hidden; the choice is kept for later.
function show(tab: Tab) {
  wanted = tab;
  storage.set("tab", tab);
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

show(wanted);

let signedIn = false;
const signIn = $<HTMLButtonElement>("sign-in");
signIn.addEventListener("click", () => {
  if (signedIn) return void Service.SignOut();
  signIn.disabled = true;
  signIn.textContent = "Signing in…";
  // Go shows why sign-in failed in the status bar.
  void Service.SignIn().catch(() => undefined).finally(() => {
    signIn.disabled = false;
    signIn.textContent = signedIn ? "Sign out" : "Sign in with Google";
  });
});

// Go shows the download's progress and any error in the status bar.
$("update").addEventListener("click", () => void Service.Update().catch(() => undefined));

const live = setupLive();
const wizard = setupWizard();
const moveOverlay = setupMoveOverlay();
const settings = setupSettings();
const updates = setupUpdates();
let theme = "";

onState((s: State) => {
  const nextTheme = s.dark ? "dark" : "light";
  if (nextTheme !== theme) {
    theme = nextTheme;
    document.documentElement.dataset.theme = theme;
    recorder.shown(); // canvases read the theme colours
  }
  if (s.recorder !== recorderShown) {
    recorderShown = s.recorder;
    show(wanted);
  }
  live(s);
  wizard(s);
  moveOverlay(s);
  if (recorderShown) recorder.onState(s);
  settings(s);
  updates(s);

  signedIn = s.account !== "";
  $("account").hidden = !signedIn;
  $("account").textContent = signedIn ? `${s.account} · ${s.role || "no role"}` : "";
  signIn.hidden = !s.signIn;
  if (!signIn.disabled) signIn.textContent = signedIn ? "Sign out" : "Sign in with Google";

  const update = $<HTMLButtonElement>("update");
  update.hidden = s.update === "";
  update.disabled = s.updating || s.running || s.recording || s.uploading !== "";
  update.textContent = s.updating ? `Updating to ${s.update}…` : `Update to ${s.update}`;

  $("status").textContent = s.status;
  $("version").textContent = s.version;
  $("dry-run").hidden = !s.dryRun;
  $("stop-key").textContent = keyLabel(s.stopKey);
  $("stop-hint").hidden = !s.stopKey;
});
