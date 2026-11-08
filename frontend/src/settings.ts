// Settings: every change saves at once, and Go validates and clamps it. Keys
// are set by pressing them, numbers with sliders, techniques with dropdowns.
import { Service, type Settings, type SettingsForm, type State } from "./api";
import { $, h, keyLabel } from "./dom";

type Key = keyof Settings;
type Value = Settings[Key];

type Field = { key: Key; label: string; hint: string } & (
  | { kind: "key" | "switch" | "text" | "locator" | "biteDetector" }
  | { kind: "slider"; min: number; max: number; step: number; unit?: string }
);

/** User groups come first; dev groups (detection tuning) show only in debug mode. */
const groups: { title: string; dev?: boolean; fields: Field[] }[] = [
  {
    title: "Keys",
    fields: [
      { key: "castKey", label: "Cast key", hint: "Click to set the key or mouse side button you use to cast Fishing in game.", kind: "key" },
      { key: "stopKey", label: "Stop key", hint: "Stops fishing from anywhere, applies after a restart", kind: "key" },
    ],
  },
  {
    title: "Run",
    fields: [
      { key: "maxSessionMinutes", label: "Session length", hint: "Stop at the first cast after this long", kind: "slider", min: 5, max: 240, step: 5, unit: "min" },
      { key: "maxFailedCasts", label: "Casts without a bite", hint: "Stop after this many casts in a row without a catch", kind: "slider", min: 1, max: 20, step: 1 },
      { key: "cooldownSeconds", label: "Pause after a catch", hint: "Wait before the next cast", kind: "slider", min: 1, max: 60, step: 1, unit: "s" },
      { key: "maxCastDurationSeconds", label: "Cast length", hint: "Recast when no bite comes within this time", kind: "slider", min: 5, max: 180, step: 5, unit: "s" },
    ],
  },
  {
    title: "Detection",
    dev: true,
    fields: [
      { key: "locator", label: "Locator", hint: "Finds the bobber in the game window", kind: "locator" },
      { key: "biteDetector", label: "Bite detector", hint: "Decides when a fish bites", kind: "biteDetector" },
      { key: "roiSizePx", label: "Watch area", hint: "Side of the square watched for a bite (ncc locator)", kind: "slider", min: 32, max: 256, step: 8, unit: "px" },
    ],
  },
  {
    title: "Template search",
    dev: true,
    fields: [
      { key: "minScale", label: "Min scale", hint: "Smallest template size tried", kind: "slider", min: 0.5, max: 2, step: 0.05 },
      { key: "maxScale", label: "Max scale", hint: "Largest template size tried", kind: "slider", min: 0.5, max: 2, step: 0.05 },
      { key: "scaleStep", label: "Scale step", hint: "Size change between tries", kind: "slider", min: 0.01, max: 0.2, step: 0.01 },
      { key: "threshold", label: "Threshold", hint: "Match score needed", kind: "slider", min: 0.5, max: 0.99, step: 0.01 },
      { key: "stride", label: "Stride", hint: "Pixels between search positions", kind: "slider", min: 1, max: 16, step: 1, unit: "px" },
      { key: "refine", label: "Refine", hint: "Search around the best match at every pixel", kind: "switch" },
    ],
  },
  {
    title: "Recording",
    dev: true,
    fields: [
      { key: "recordDir", label: "Recording folder", hint: "Each recording gets a new folder in here", kind: "text" },
      { key: "recordKeys", label: "Record keys", hint: "Record keys as well as the mouse", kind: "switch" },
    ],
  },
];

/** keyName reads a supported unmodified keyboard binding. */
function keyName(e: KeyboardEvent): string | undefined {
  if (e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return undefined;
  const key = e.key.toUpperCase();
  if (/^[A-Z]$/.test(key) || /^F([1-9]|1[0-2])$/.test(key)) return key;
  if (/^[0-9]$/.test(key) && /^Digit[0-9]$/.test(e.code)) return key;
  return undefined;
}

/** sideButton names the mouse side buttons, the only mouse buttons a key can be. */
const sideButton: Record<number, string> = { 3: "Mouse4", 4: "Mouse5" };


/** A control shows one setting and reports changes the person makes. */
interface Control {
  el: HTMLElement;
  input: HTMLElement; // what the label points at
  get(): Value;
  set(v: Value): void;
  enable(on: boolean): void;
}

export function setupSettings(): (s: State) => void {
  const form = $<HTMLFormElement>("settings");
  const controls = new Map<Key, Control>();
  const devSections: HTMLElement[] = [];
  const message = h("p", "Changes save as you make them", "settings-status");
  let running = false;
  let debug = false;
  let saving = Promise.resolve();
  let savedValues: Settings | undefined;

  function setMessage(text: string, bad = false) {
    message.textContent = text;
    message.classList.toggle("bad", bad);
  }

  // save sends the whole form; Go answers with the values as stored, clamped.
  function save() {
    const values = Object.fromEntries([...controls].map(([k, c]) => [k, c.get()])) as unknown as Settings;
    saving = saving.then(async () => {
      try {
        const stored = await Service.SaveSettings(values);
        savedValues = stored;
        for (const [k, c] of controls) c.set(stored[k]);
        setMessage("Saved");
      } catch (e) {
        if (savedValues) {
          for (const [k, c] of controls) c.set(savedValues[k]);
        }
        setMessage(String(e).replace(/^Error: /, ""), true);
      }
    });
  }

  function keyControl(f: Field): Control {
    const button = h("button", "", "key-button");
    button.type = "button";
    let value = "";
    let listening = false;
    const show = () => {
      button.classList.toggle("listening", listening);
      button.replaceChildren(listening ? "Press a key or button" : h("kbd", keyLabel(value)));
    };
    const stop = () => {
      listening = false;
      removeEventListener("keydown", onKey, true);
      removeEventListener("mousedown", onButton, true);
      removeEventListener("mouseup", swallow, true);
      show();
    };
    const take = (name: string) => {
      value = name;
      stop();
      save();
    };
    // Side buttons also mean back and forward to the page, so both edges are kept from it.
    const swallow = (e: MouseEvent) => {
      if (e.button in sideButton) e.preventDefault();
    };
    const onButton = (e: MouseEvent) => {
      const name = sideButton[e.button];
      if (!name) return; // a left click elsewhere ends listening through blur
      e.preventDefault();
      e.stopPropagation();
      if (e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) {
        setMessage("Use a mouse side button without modifiers", true);
        return;
      }
      take(name);
    };
    const onKey = (e: KeyboardEvent) => {
      e.preventDefault();
      e.stopPropagation();
      if (e.key === "Escape") return stop();
      const name = keyName(e);
      if (!name) {
        setMessage(`Use a letter, digit, F1 to F12, Mouse 4 or Mouse 5 without modifiers for the ${f.label.toLowerCase()}`, true);
        return;
      }
      take(name);
    };
    button.addEventListener("click", () => {
      if (listening) return stop();
      listening = true;
      setMessage(`Press the new ${f.label.toLowerCase()} or mouse side button, or Esc to keep ${keyLabel(value)}`);
      addEventListener("keydown", onKey, true);
      addEventListener("mousedown", onButton, true);
      addEventListener("mouseup", swallow, true);
      show();
    });
    button.addEventListener("blur", () => listening && stop());
    return {
      el: button,
      input: button,
      get: () => value,
      set: (v) => { value = String(v); show(); },
      enable: (on) => { button.disabled = !on; if (!on && listening) stop(); },
    };
  }

  function sliderControl(f: Extract<Field, { kind: "slider" }>): Control {
    const box = h("div", "", "slider");
    const range = document.createElement("input");
    range.type = "range";
    range.min = String(f.min);
    range.max = String(f.max);
    range.step = String(f.step);
    const out = h("output", "", "mono");
    const decimals = String(f.step).split(".")[1]?.length ?? 0;
    const show = () => (out.textContent = `${Number(range.value).toFixed(decimals)}${f.unit ? ` ${f.unit}` : ""}`);
    range.addEventListener("input", show);
    range.addEventListener("change", save); // on release, not on every step
    box.append(range, out);
    return {
      el: box,
      input: range,
      get: () => Number(range.value),
      set: (v) => { range.value = String(v); show(); },
      enable: (on) => { range.disabled = !on; },
    };
  }

  function switchControl(): Control {
    const input = document.createElement("input");
    input.type = "checkbox";
    input.className = "switch";
    input.addEventListener("change", save);
    return {
      el: input,
      input,
      get: () => input.checked,
      set: (v) => { input.checked = Boolean(v); },
      enable: (on) => { input.disabled = !on; },
    };
  }

  function selectControl(names: string[]): Control {
    const select = document.createElement("select");
    select.append(...names.map((n) => new Option(n, n)));
    select.addEventListener("change", save);
    return {
      el: select,
      input: select,
      get: () => select.value,
      set: (v) => { select.value = String(v); },
      enable: (on) => { select.disabled = !on; },
    };
  }

  function textControl(): Control {
    const input = document.createElement("input");
    input.type = "text";
    input.addEventListener("change", save); // on Enter or leaving the field
    return {
      el: input,
      input,
      get: () => input.value.trim(),
      set: (v) => { input.value = String(v); },
      enable: (on) => { input.disabled = !on; },
    };
  }

  function control(f: Field, data: SettingsForm): Control {
    switch (f.kind) {
      case "key": return keyControl(f);
      case "slider": return sliderControl(f);
      case "switch": return switchControl();
      case "text": return textControl();
      case "locator": return selectControl(data.locators ?? []);
      case "biteDetector": return selectControl(data.biteDetectors ?? []);
    }
  }

  function row(id: string, labelText: string, hint: string, el: HTMLElement, input: HTMLElement) {
    const r = h("div", "", "row");
    input.id = id;
    const label = h("label");
    label.htmlFor = id;
    label.append(labelText, h("span", hint, "hint"));
    const box = h("div", "", "control");
    box.append(el);
    r.append(label, box);
    return r;
  }

  const dark = document.createElement("input");
  dark.type = "checkbox";
  dark.className = "switch";
  dark.addEventListener("change", () => void Service.SetDark(dark.checked));

  function lock() {
    for (const c of controls.values()) c.enable(!running);
    if (running) setMessage("Stop the run to change settings");
    else if (message.textContent === "Stop the run to change settings") setMessage("Changes save as you make them");
  }

  async function build() {
    const data = await Service.Settings();
    savedValues = data.values;
    const sections = groups.map((g) => {
      const fs = h("fieldset");
      fs.append(h("legend", g.title));
      if (g.dev) devSections.push(fs);
      fs.hidden = Boolean(g.dev) && !debug;
      for (const f of g.fields) {
        const c = control(f, data);
        c.set(data.values[f.key]);
        controls.set(f.key, c);
        fs.append(row(`setting-${f.key}`, f.label, f.hint, c.el, c.input));
      }
      return fs;
    });
    const look = h("fieldset");
    look.append(h("legend", "Appearance"), row("setting-dark", "Dark theme", "Graphite surfaces, easier on the eyes at night", dark, dark));
    form.replaceChildren(message, ...sections.slice(0, 2), look, ...sections.slice(2));
    lock();
  }

  form.addEventListener("submit", (e) => e.preventDefault()); // Enter in the text field must not reload
  void build();

  return (s) => {
    dark.checked = s.dark;
    if (s.debug !== debug) {
      debug = s.debug;
      for (const fs of devSections) fs.hidden = !debug;
    }
    if (s.running === running) return;
    running = s.running;
    lock();
  };
}
