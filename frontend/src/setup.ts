// Setup panel: what the bot still needs before it can fish, with a button for
// each thing Go can do. Go decides the steps (ui/desktop/setup.go); this only
// shows them, and the panel goes away once nothing is missing.
import { Check } from "lucide";
import { Service, type State } from "./api";
import { $, h, setIcon } from "./dom";

type Step = State["setup"][number];

/** What a step's button does; its label comes from Go. */
const actions: Record<string, () => Promise<unknown>> = {
  addon: () => Service.InstallAddon(),
  account: () => Service.SignIn(),
};

/** How often to look for the game, which may start after Go Fish. */
const CHECK_MS = 4000;

export function setupWizard(): (state: State) => void {
  const panel = $("setup");
  const list = $("setup-list");
  let shown = "";

  // Go decides what a change means: it selects the game when it opens and
  // rechecks the addon, which both show up in the next state.
  const check = () => {
    if (!document.hidden) void Service.CheckGame();
  };
  setInterval(check, CHECK_MS);
  check();
  $("setup-check").addEventListener("click", check);

  function row(step: Step): HTMLElement {
    const li = h("li", "", `step${step.done ? " done" : ""}`);
    const mark = h("span", "", "mark");
    mark.setAttribute("aria-hidden", "true");
    if (step.done) setIcon(mark, Check);
    const title = h("span", step.title, "title");
    li.append(mark, title);
    if (step.detail) li.append(h("span", step.detail, "detail"));
    if (step.action) {
      const button = h("button", step.action, "button primary");
      button.type = "button";
      button.addEventListener("click", () => {
        button.disabled = true;
        // Go shows why an action failed in the status bar.
        void (actions[step.id]?.() ?? Promise.resolve())
          .catch(() => undefined)
          .finally(() => {
            button.disabled = false;
            check();
          });
      });
      li.append(button);
    }
    return li;
  }

  return (s) => {
    panel.hidden = s.setupDone;
    // Rebuild only on a change, so a click is not lost to a redraw.
    const key = JSON.stringify(s.setup);
    if (s.setupDone || key === shown) return;
    shown = key;
    list.replaceChildren(...s.setup.filter((step) => step.id !== "").map(row));
  };
}
