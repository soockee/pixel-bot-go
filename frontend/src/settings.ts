// Settings: every change saves at once, and Go validates and clamps it. Keys
// are set by pressing them, numbers with sliders, techniques with dropdowns.
import { Service, type Settings, type SettingsForm, type State } from "./api";
import { $, h, keyLabel } from "./dom";
import { playSound, sounds, themes } from "./themes";

type Key = keyof Settings;
type Value = Settings[Key];

/** when hides a field the other values make pointless; optional keys can be cleared. */
type Field = { key: Key; label: string; hint: string; when?: (v: Settings) => boolean; optional?: boolean } & (
  | { kind: "key" | "switch" | "text" | "port" | "locator" | "biteDetector" | "reelMethod" }
  | { kind: "slider"; min: number; max: number; step: number; unit?: string; zero?: string }
);

/** User groups come first; dev groups (detection tuning) show only in debug
 * mode, recorder groups while the Recorder is shown. */
const breaks = (v: Settings) => v.schedule;
const logout = (v: Settings) => v.schedule || v.reposition;

const groups: { title: string; show?: "debug" | "recorder"; fields: Field[] }[] = [
  {
    title: "Fishing keys",
    fields: [
      { key: "castKey", label: "Cast key", hint: "Choose a key or mouse side button. Bobber Marker binds it to Fishing in WoW.", kind: "key" },
    ],
  },
  {
    title: "Reeling",
    fields: [
      { key: "interactKey", label: "Interact key", hint: "Optional. Set it to the same binding as Interact With Target in WoW to unlock the Interact reel method.", kind: "key" },
      { key: "reelMethod", label: "Reel method", hint: "Right-click the bobber, or press the game's Interact key without moving the cursor", kind: "reelMethod" },
    ],
  },
  {
    title: "Input",
    fields: [
      { key: "esp32Port", label: "ESP32 board", hint: "Send every key, click and cursor move through the paired board. Auto finds it on its USB cable. Off uses Windows input.", kind: "port" },
    ],
  },
  {
    title: "Run",
    fields: [
      { key: "maxSessionMinutes", label: "Session length", hint: "Stop at the first cast after this long", kind: "slider", min: 5, max: 240, step: 5, unit: "min" },
      { key: "maxFailedCasts", label: "Casts without a bite", hint: "Stop after this many casts in a row without a catch", kind: "slider", min: 1, max: 20, step: 1 },
      { key: "cooldownSeconds", label: "Pause after a catch", hint: "Wait before the next cast. Quick casts again after 0.4 to 1.2 s", kind: "slider", min: 0, max: 60, step: 1, unit: "s", zero: "Quick" },
      { key: "maxCastDurationSeconds", label: "Cast length", hint: "Recast when no bite comes within this time", kind: "slider", min: 5, max: 180, step: 5, unit: "s" },
    ],
  },
  {
    title: "Between catches",
    fields: [
      { key: "idleMoves", label: "Move the mouse", hint: "Now and then, drift the cursor a little or nudge the camera", kind: "switch" },
      { key: "reposition", label: "Change spot", hint: "Every 45 to 55 catches, take a step with WASD or turn the camera, then check the next cast. Logs out if no spot works.", kind: "switch" },
      { key: "targetPlayerKey", label: "Target passing player", hint: "An unused key Bobber Marker binds to select a friendly player who walks by. Update the addon after setting it.", kind: "key", optional: true },
    ],
  },
  {
    title: "Scheduled breaks",
    fields: [
      { key: "schedule", label: "Take breaks", hint: "Alternate random fishing sessions with logged-out breaks", kind: "switch" },
      { key: "playMinMinutes", label: "Shortest session", hint: "Lower end of the random play time", kind: "slider", min: 1, max: 240, step: 1, unit: "min", when: breaks },
      { key: "playMaxMinutes", label: "Longest session", hint: "Upper end of the random play time", kind: "slider", min: 1, max: 240, step: 1, unit: "min", when: breaks },
      { key: "breakMinMinutes", label: "Shortest break", hint: "Lower end of the random break", kind: "slider", min: 1, max: 240, step: 1, unit: "min", when: breaks },
      { key: "breakMaxMinutes", label: "Longest break", hint: "Upper end of the random break", kind: "slider", min: 1, max: 240, step: 1, unit: "min", when: breaks },
      { key: "logoutKey", label: "Logout key", hint: "A spare key. Bobber Marker puts a Logout macro on it; update the addon and type /reload after setting it.", kind: "key", when: logout },
    ],
  },
  {
    title: "Data sharing",
    fields: [
      { key: "shareRecordings", label: "Share recordings", hint: "Shows the Recorder, to record yourself fishing and send it in to improve detection. Nothing is sent until you click Upload.", kind: "switch" },
    ],
  },
  {
    title: "Recording",
    show: "recorder",
    fields: [
      { key: "recordDir", label: "Recording folder", hint: "Each recording gets a new folder in here", kind: "text" },
      { key: "recordKeys", label: "Record keys", hint: "Record keys as well as the mouse, including what you type in chat", kind: "switch" },
    ],
  },
  {
    title: "Detection",
    show: "debug",
    fields: [
      { key: "locator", label: "Locator", hint: "Finds the bobber in the game window", kind: "locator" },
      { key: "biteDetector", label: "Bite detector", hint: "Decides when a fish bites", kind: "biteDetector" },
      { key: "roiSizePx", label: "Watch area", hint: "Side of the square watched for a bite (ncc locator)", kind: "slider", min: 32, max: 256, step: 8, unit: "px" },
    ],
  },
  {
    title: "Template search",
    show: "debug",
    fields: [
      { key: "minScale", label: "Min scale", hint: "Smallest template size tried", kind: "slider", min: 0.5, max: 2, step: 0.05 },
      { key: "maxScale", label: "Max scale", hint: "Largest template size tried", kind: "slider", min: 0.5, max: 2, step: 0.05 },
      { key: "scaleStep", label: "Scale step", hint: "Size change between tries", kind: "slider", min: 0.01, max: 0.2, step: 0.01 },
      { key: "threshold", label: "Threshold", hint: "Match score needed", kind: "slider", min: 0.5, max: 0.99, step: 0.01 },
      { key: "stride", label: "Stride", hint: "Pixels between search positions", kind: "slider", min: 1, max: 16, step: 1, unit: "px" },
      { key: "refine", label: "Refine", hint: "Search around the best match at every pixel", kind: "switch" },
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

export function setupSettings() {
  const form = $<HTMLFormElement>("settings");
  const controls = new Map<Key, Control>();
  const conditional: { row: HTMLElement; when: (v: Settings) => boolean }[] = [];
  const values = () => Object.fromEntries([...controls].map(([k, c]) => [k, c.get()])) as unknown as Settings;
  const refresh = (v: Settings) => {
    for (const c of conditional) c.row.hidden = !c.when(v);
    const reel = controls.get("reelMethod")?.input;
    const option = reel?.querySelector<HTMLOptionElement>('option[value="interact_key"]');
    if (option) option.disabled = !v.interactKey && v.reelMethod !== "interact_key";
  };
  const gated: { fs: HTMLElement; show: "debug" | "recorder" }[] = [];
  const message = h("p", "Changes save as you make them", "settings-status");
  let running = false;
  let shown = { debug: false, recorder: false };
  let saving = Promise.resolve();
  let saveFailed = false;
  let savedValues: Settings | undefined;
  let castRow: HTMLElement | undefined;
  let castHome: HTMLElement | undefined;
  let castSlot: HTMLElement | undefined;

  function placeCastKey() {
    if (!castRow || !castHome) return;
    const parent = castSlot ?? castHome;
    if (castRow.parentElement === parent) return;
    if (castSlot) {
      castSlot.append(castRow, message);
    } else {
      castHome.firstElementChild?.after(castRow);
      form.prepend(message);
    }
  }

  function setMessage(text: string, bad = false) {
    message.textContent = text;
    message.classList.toggle("bad", bad);
  }

  // save sends the whole form; Go answers with the values as stored, clamped.
  function save() {
    const next = values();
    refresh(next);
    saving = saving.then(async () => {
      try {
        const stored = await Service.SaveSettings(next);
        saveFailed = false;
        savedValues = stored;
        for (const [k, c] of controls) c.set(stored[k]);
        setMessage("Saved");
      } catch (e) {
        saveFailed = true;
        if (savedValues) {
          for (const [k, c] of controls) c.set(savedValues[k]);
        }
        setMessage(String(e).replace(/^Error: /, ""), true);
      }
      refresh(values());
    });
  }

  function keyControl(f: Field): Control {
    const button = h("button", "", "key-button");
    button.type = "button";
    let value = "";
    let listening = false;
    const show = () => {
      button.classList.toggle("listening", listening);
      button.replaceChildren(listening ? "Press a key or button" : value ? h("kbd", keyLabel(value)) : f.optional ? "Off" : "Set key");
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
      setMessage(`Press the new ${f.label.toLowerCase()} or mouse side button, or Esc to cancel`);
      addEventListener("keydown", onKey, true);
      addEventListener("mousedown", onButton, true);
      addEventListener("mouseup", swallow, true);
      show();
    });
    button.addEventListener("blur", () => listening && stop());
    if (!f.optional) {
      return {
        el: button,
        input: button,
        get: () => value,
        set: (v) => { value = String(v); show(); },
        enable: (on) => { button.disabled = !on; if (!on && listening) stop(); },
      };
    }
    const clear = h("button", "Clear", "button");
    clear.type = "button";
    clear.addEventListener("click", () => take(""));
    const box = h("div", "", "key-pair");
    box.append(button, clear);
    return {
      el: box,
      input: button,
      get: () => value,
      set: (v) => { value = String(v); show(); clear.hidden = !value; },
      enable: (on) => { button.disabled = clear.disabled = !on; if (!on && listening) stop(); },
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
    const show = () =>
      (out.textContent = f.zero && Number(range.value) === 0 ? f.zero : `${Number(range.value).toFixed(decimals)}${f.unit ? ` ${f.unit}` : ""}`);
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

  function selectControl(names: string[], labels: Record<string, string> = {}): Control {
    const select = document.createElement("select");
    select.append(...names.map((n) => new Option(labels[n] ?? n, n)));
    select.addEventListener("change", save);
    return {
      el: select,
      input: select,
      get: () => select.value,
      set: (v) => { select.value = String(v); },
      enable: (on) => { select.disabled = !on; },
    };
  }

  // The ports are listed again on each open: the board may be plugged in since.
  function portControl(): Control {
    const select = document.createElement("select");
    let value = "";
    async function load() {
      const ports = (await Service.ESP32Ports()) ?? [];
      select.replaceChildren(new Option("Off, use Windows input", ""), new Option("Auto, find the board", "AUTO"), ...ports.map((p) => new Option(p, p)));
      select.value = value;
    }
    select.addEventListener("mousedown", () => void load());
    select.addEventListener("change", () => {
      value = select.value;
      save();
    });
    void load();
    return {
      el: select,
      input: select,
      get: () => select.value,
      set: (v) => {
        value = String(v);
        if (![...select.options].some((o) => o.value === value)) select.add(new Option(value, value));
        select.value = value;
      },
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
      case "port": return portControl();
      case "reelMethod": return selectControl(["right_click", "interact_key"], { right_click: "Right click", interact_key: "Interact key" });
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

  // Each theme is a card with its icon; picking one plays its catch sound.
  const picker = h("div", "", "theme-picker");
  picker.setAttribute("role", "radiogroup");
  picker.setAttribute("aria-label", "Theme");
  const choices = themes.map((t) => {
    const input = document.createElement("input");
    input.type = "radio";
    input.name = "theme";
    input.value = t.id;
    input.addEventListener("change", () => {
      void Service.SetTheme(t.id);
      playSound(t);
    });
    const icon = document.createElement("img");
    icon.src = `/themes/${t.id}.svg`;
    icon.alt = "";
    const card = h("label", "", "theme-card");
    card.dataset.preview = t.id;
    const text = h("span", "", "theme-text");
    text.append(h("strong", t.name), h("span", t.blurb, "hint"));
    card.append(input, icon, text);
    picker.append(card);
    return input;
  });
  const sound = document.createElement("input");
  sound.type = "checkbox";
  sound.className = "switch";
  sound.checked = sounds.on();
  sound.addEventListener("change", () => sounds.set(sound.checked));

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
      if (g.show) gated.push({ fs, show: g.show });
      fs.hidden = Boolean(g.show) && !shown[g.show!];
      for (const f of g.fields) {
        const c = control(f, data);
        c.set(data.values[f.key]);
        controls.set(f.key, c);
        const r = row(`setting-${f.key}`, f.label, f.hint, c.el, c.input);
        if (f.key === "castKey") {
          castRow = r;
          castHome = fs;
        }
        if (f.when) conditional.push({ row: r, when: f.when });
        fs.append(r);
      }
      return fs;
    });
    refresh(data.values);
    const look = h("fieldset");
    look.append(h("legend", "Theme"), picker, row("setting-sounds", "Theme sounds", "Each theme greets a catch with its own sound", sound, sound));
    const user = groups.findIndex((g) => g.title === "Data sharing");
    form.replaceChildren(message, ...sections.slice(0, user), look, ...sections.slice(user));
    placeCastKey();
    lock();
  }

  form.addEventListener("submit", (e) => e.preventDefault()); // Enter in the text field must not reload
  const built = build();

  function render(s: State) {
    for (const c of choices) c.checked = c.value === s.theme;
    if (s.debug !== shown.debug || s.recorder !== shown.recorder) {
      shown = { debug: s.debug, recorder: s.recorder };
      for (const g of gated) g.fs.hidden = !shown[g.show];
    }
    if (s.running === running) return;
    running = s.running;
    lock();
  }

  return {
    render,
    mountCastKey(slot?: HTMLElement) {
      castSlot = slot;
      placeCastKey();
    },
    async finishSaving() {
      await built;
      await saving;
      if (saveFailed) {
        saveFailed = false;
        throw new Error(message.textContent || "Could not save fishing keys");
      }
    },
  };
}
