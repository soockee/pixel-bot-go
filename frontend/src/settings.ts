// Settings tab: the editable config, grouped, with Go doing the validation.
import { Service, type Settings, type SettingsForm, type State } from "./api";
import { $, h } from "./dom";

type Key = keyof Settings;
interface Field {
  key: Key;
  label: string;
  hint: string;
  kind: "number" | "text" | "bool" | "locator" | "biteDetector";
  step?: number;
}

const groups: { title: string; fields: Field[] }[] = [
  {
    title: "Detection",
    fields: [
      { key: "locator", label: "Locator", hint: "Finds the bobber in the capture area", kind: "locator" },
      { key: "biteDetector", label: "Bite detector", hint: "Decides when a fish bites", kind: "biteDetector" },
      { key: "roiSizePx", label: "Watch area", hint: "Side of the square watched for a bite, 32 to 256 px", kind: "number", step: 1 },
    ],
  },
  {
    title: "Template search",
    fields: [
      { key: "minScale", label: "Min scale", hint: "Smallest template size tried", kind: "number", step: 0.01 },
      { key: "maxScale", label: "Max scale", hint: "Largest template size tried", kind: "number", step: 0.01 },
      { key: "scaleStep", label: "Scale step", hint: "Size change between tries", kind: "number", step: 0.005 },
      { key: "threshold", label: "Threshold", hint: "Match score needed, 0 to 1", kind: "number", step: 0.01 },
      { key: "stride", label: "Stride", hint: "Pixels between search positions", kind: "number", step: 1 },
      { key: "refine", label: "Refine", hint: "Search around the best match at every pixel", kind: "bool" },
    ],
  },
  {
    title: "Keys",
    fields: [
      { key: "castKey", label: "Cast key", hint: "A to Z or F1 to F12", kind: "text" },
      { key: "stopKey", label: "Stop hotkey", hint: "Stops a run or recording from anywhere, applies after a restart", kind: "text" },
    ],
  },
  {
    title: "Limits",
    fields: [
      { key: "maxFailedCasts", label: "Failed casts", hint: "Stop after this many casts in a row without a reel", kind: "number", step: 1 },
      { key: "maxSessionMinutes", label: "Session length", hint: "Stop at the first cast after this many minutes", kind: "number", step: 1 },
      { key: "maxCastDurationSeconds", label: "Cast length", hint: "Recast when no bite comes within this many seconds, 5 to 180", kind: "number", step: 1 },
      { key: "cooldownSeconds", label: "Cooldown", hint: "Seconds to wait after reeling, 1 to 60", kind: "number", step: 1 },
    ],
  },
  {
    title: "Recording",
    fields: [
      { key: "recordDir", label: "Recording folder", hint: "Each recording gets a new folder in here", kind: "text" },
      { key: "recordKeys", label: "Record keys", hint: "Record keys as well as the mouse", kind: "bool" },
    ],
  },
];

export function setupSettings(): (s: State) => void {
  const form = $<HTMLFormElement>("settings");
  const controls = new Map<Key, HTMLInputElement | HTMLSelectElement>();
  const save = h("button", "Save", "button primary");
  save.type = "submit";
  const message = h("span", "", "message");
  let running = false;

  function control(f: Field, data: SettingsForm): HTMLInputElement | HTMLSelectElement {
    if (f.kind === "locator" || f.kind === "biteDetector") {
      const select = document.createElement("select");
      const names = (f.kind === "locator" ? data.locators : data.biteDetectors) ?? [];
      select.append(...names.map((n) => new Option(n, n)));
      return select;
    }
    const input = document.createElement("input");
    input.type = f.kind === "bool" ? "checkbox" : f.kind;
    if (f.step) input.step = String(f.step);
    return input;
  }

  function fill(values: Settings) {
    for (const [key, el] of controls) {
      const v = values[key];
      if (el instanceof HTMLInputElement && el.type === "checkbox") el.checked = Boolean(v);
      else el.value = String(v);
    }
  }

  function read(): Settings {
    const values: Record<string, unknown> = {};
    for (const [key, el] of controls) {
      values[key] = el instanceof HTMLInputElement && el.type === "checkbox" ? el.checked
        : el instanceof HTMLInputElement && el.type === "number" ? Number(el.value)
        : el.value.trim();
    }
    return values as unknown as Settings;
  }

  function lock() {
    for (const el of controls.values()) el.disabled = running;
    save.disabled = running;
    if (running) setMessage("Stop the run to change settings");
    else if (message.textContent === "Stop the run to change settings") setMessage("");
  }

  function setMessage(text: string, bad = false) {
    message.textContent = text;
    message.classList.toggle("bad", bad);
  }

  async function build() {
    const data = await Service.Settings();
    const sections = groups.map((g) => {
      const fs = h("fieldset");
      fs.append(h("legend", g.title));
      for (const f of g.fields) {
        const id = `setting-${f.key}`;
        const el = control(f, data);
        el.id = id;
        controls.set(f.key, el);
        const row = h("div", "", "row");
        const label = h("label");
        label.htmlFor = id;
        label.append(f.label, h("span", f.hint, "hint"));
        const box = h("div", "", "control");
        box.append(el);
        row.append(label, box);
        fs.append(row);
      }
      return fs;
    });
    const bar = h("div", "", "savebar");
    bar.append(message, save);
    form.replaceChildren(...sections, bar);
    fill(data.values);
    lock();
  }

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    try {
      fill(await Service.SaveSettings(read()));
      setMessage("Saved");
    } catch (e) {
      setMessage(String(e).replace(/^Error: /, ""), true);
    }
  });
  form.addEventListener("input", () => setMessage("Unsaved changes"));

  void build();

  return (s) => {
    if (s.running === running) return;
    running = s.running;
    lock();
  };
}
