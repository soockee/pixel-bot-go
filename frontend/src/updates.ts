// Updates and Addon panels on the Settings tab: the running version, the latest release
// and when the app last asked GitHub. Go checks, verifies and installs.
import { Info, RefreshCw } from "lucide";
import { Service, type State } from "./api";
import { $, setIcon } from "./dom";

/** ago reads Unix seconds as a short relative time. */
function ago(unix: number): string {
  const s = Date.now() / 1000 - unix;
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)} min ago`;
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`;
  const days = Math.floor(s / 86400);
  return days === 1 ? "yesterday" : `${days} days ago`;
}

export function setupUpdates(): (s: State) => void {
  setIcon($("check-icon"), RefreshCw);
  setIcon($("warning-icon"), Info);
  // Go shows why a check or an update failed, in this panel and the status bar.
  $("check-update").addEventListener("click", () => void Service.CheckUpdate().catch(() => undefined));
  $("install-update").addEventListener("click", () => void Service.Update().catch(() => undefined));
  $("release-notes").addEventListener("click", () => void Service.ReleaseNotes());

  setIcon($("check-addon-icon"), RefreshCw);
  $("check-addon").addEventListener("click", () => void Service.CheckAddon());
  // Go shows why an install failed in the status bar.
  $("install-addon").addEventListener("click", () => void Service.InstallAddon().catch(() => undefined));

  let last: State | undefined;
  function render(s: State) {
    last = s;
    const checked = $("update-checked");
    checked.classList.toggle("bad", s.updateError !== "" && s.updateChecked !== 0);
    checked.textContent = s.checking ? "Checking…"
      : s.updateError ? (s.updateChecked ? `Check failed ${ago(s.updateChecked)}: ${s.updateError}` : s.updateError)
      : s.updateChecked ? `Checked ${ago(s.updateChecked)}` : "Not checked yet";
    $<HTMLButtonElement>("check-update").disabled = s.checking || s.updating;

    const offered = s.update !== "";
    $("update-detail").textContent = !s.latest ? s.version
      : offered ? `${s.version} installed · ${s.latest} released ${ago(s.latestReleased)} · `
      : `${s.version} · released ${ago(s.latestReleased)} · `;
    $("release-notes").hidden = !s.latest;

    const busy = s.running ? "Stop the run before updating."
      : s.recording ? "Stop the recording before updating." : "";
    $("update-warning").hidden = !offered || !busy || s.updating;
    $("update-warning-text").textContent = busy;

    const install = $<HTMLButtonElement>("install-update");
    install.hidden = !offered;
    install.disabled = s.updating || busy !== "";
    install.textContent = s.updating ? `Updating to ${s.update}…` : `Update to ${s.update}`;

    const pill = $("update-pill");
    const upToDate = !offered && s.latest !== "" && !s.updateError;
    pill.hidden = offered || (!upToDate && !s.checking);
    pill.textContent = s.checking ? "Checking" : "Up to date";
    pill.classList.toggle("good", upToDate && !s.checking);
    renderAddon(s);
  }

  function renderAddon(s: State) {
    $("addon-where").textContent = s.addonDir ? s.addonDir : "Start World of Warcraft: Forever, then check again";
    $("addon-detail").textContent = s.addon === "outdated" ? `v${s.addonVersion} ready · an older version is installed`
      : s.addon === "missing" ? `v${s.addonVersion} · not installed` : `v${s.addonVersion}`;
    const install = $<HTMLButtonElement>("install-addon");
    install.hidden = s.addon !== "missing" && s.addon !== "outdated";
    install.textContent = s.addon === "outdated" ? "Update" : "Install";
    const pill = $("addon-pill");
    pill.hidden = s.addon !== "current";
    pill.textContent = "Up to date";
    pill.classList.add("good");
  }
  setInterval(() => last && render(last), 30_000); // keeps "… ago" current
  return render;
}
