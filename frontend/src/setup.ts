// Setup panel: what the bot still needs before it can fish, with a button for
// each thing Go can do. Go decides the steps (ui/desktop/setup.go); this only
// shows them, and the panel goes away once nothing is missing.
import { Check } from "lucide";
import { Service, type State } from "./api";
import { $, h, setIcon } from "./dom";

type Step = State["setup"][number];

/** How often to look for the game, which may start after Go Fish. */
const CHECK_MS = 4000;

export function setupWizard(finishSaving: () => Promise<void>) {
  const panel = $("setup");
  const preview = $("fishing-preview");
  const list = $("setup-list");
  const progress = $("setup-progress");
  const keys = $("setup-keys");
  const install = $<HTMLButtonElement>("setup-install");
  const error = $("setup-error");
  const reload = $("setup-reload");
  let shown = "";

  // Go decides what a change means: it selects the game when it opens and
  // rechecks the addon, which both show up in the next state.
  const check = () => {
    if (!document.hidden) void Service.CheckGame().catch(() => undefined);
  };
  setInterval(check, CHECK_MS);
  $("setup-check").addEventListener("click", check);
  let activeID = "";
  install.addEventListener("click", async () => {
    install.disabled = true;
    keys.inert = true;
    error.hidden = true;
    try {
      await finishSaving();
      if (activeID === "cast") {
        await Service.ChooseCastKey();
      } else if (activeID === "addon") {
        await Service.InstallAddon();
        reload.hidden = false;
      }
    } catch (e) {
      error.textContent = String(e).replace(/^Error: /, "");
      error.hidden = false;
    } finally {
      keys.inert = false;
      install.disabled = false;
      check();
    }
  });

  function row(step: Step, index: number, active: boolean): HTMLElement {
    const li = h("li", "", `step${step.done ? " done" : ""}`);
    if (active) li.setAttribute("aria-current", "step");
    const mark = h("span", String(index + 1), "mark");
    mark.setAttribute("aria-hidden", "true");
    if (step.done) setIcon(mark, Check);
    const title = h("span", step.title, "title");
    li.append(mark, title);
    if (step.detail) li.append(h("span", step.detail, "detail"));
    return li;
  }

  function render(s: State) {
    if (s.running) reload.hidden = true;
    panel.hidden = s.setupDone || s.running;
    preview.hidden = !panel.hidden;
    const steps = s.setup.filter((step) => step.id !== "");
    const active = steps.findIndex((step) => !step.done);
    const action = steps[active]?.action;
    activeID = steps[active]?.id ?? "";
    install.hidden = panel.hidden || !action;
    keys.hidden = install.hidden || activeID !== "cast";
    if (action) install.textContent = action;
    // Rebuild only on a change, so a click is not lost to a redraw.
    const key = JSON.stringify(s.setup);
    if (s.setupDone || key === shown) return;
    shown = key;
    progress.textContent = `Step ${active + 1} of ${steps.length}`;
    list.replaceChildren(...steps.map((step, index) => row(step, index, index === active)));
  }

  return { render, keysSlot: () => keys.hidden ? undefined : keys };
}
