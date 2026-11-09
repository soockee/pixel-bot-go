// Themes change how the window looks, sounds and speaks. Go keeps the list of
// theme ids (config.Themes) and the choice; this file holds their words, the
// sound a catch makes, and the burst it throws. The look is in themes.css.
import { storage } from "./dom";

/** Words are the texts a theme renames; the cycle keys are fishing.FishingState names. */
export interface Words {
  tabs: { live: string; recorder: string; settings: string };
  start: string;
  stop: string;
  catches: [one: string, many: string];
  empty: string;
  /** support labels the coffee button and link: the theme's way of buying the maker a coffee. */
  support: string;
  cycle: Record<string, [step: string, caption: string]>;
}

export interface Theme {
  id: string;
  name: string;
  blurb: string;
  words: Words;
  /** sound plays one catch, or nothing for a quiet theme. */
  sound?: (ctx: AudioContext) => void;
  /** particles is how many pieces a catch throws over the capture. */
  particles: number;
}

const graphiteWords: Words = {
  tabs: { live: "Fishing", recorder: "Recorder", settings: "Settings" },
  start: "Start fishing",
  stop: "Stop",
  catches: ["catch this run", "catches this run"],
  empty: "Start to watch the game window here",
  support: "Buy me a coffee",
  cycle: {
    focus: ["Focus", "Waiting for the game window to have focus"],
    searching: ["Search", "Looking for the bobber"],
    monitoring: ["Watch", "Watching for a bite"],
    reeling: ["Reel", "Reeling in"],
    cooldown: ["Cool down", "Cooling down"],
    casting: ["Cast", "Casting"],
  },
};

// tone plays one shaped note: freq may glide to `to` over the note.
function tone(ctx: AudioContext, o: { type: OscillatorType; freq: number; to?: number; at?: number; len: number; gain: number; filter?: number }) {
  const t = ctx.currentTime + (o.at ?? 0);
  const osc = ctx.createOscillator();
  const amp = ctx.createGain();
  osc.type = o.type;
  osc.frequency.setValueAtTime(o.freq, t);
  if (o.to) osc.frequency.exponentialRampToValueAtTime(o.to, t + o.len);
  amp.gain.setValueAtTime(0.0001, t);
  amp.gain.exponentialRampToValueAtTime(o.gain, t + 0.01);
  amp.gain.exponentialRampToValueAtTime(0.0001, t + o.len);
  let out: AudioNode = osc;
  if (o.filter) {
    const lp = ctx.createBiquadFilter();
    lp.frequency.value = o.filter;
    osc.connect(lp);
    out = lp;
  }
  out.connect(amp).connect(ctx.destination);
  osc.start(t);
  osc.stop(t + o.len + 0.05);
}

export const themes: Theme[] = [
  {
    id: "graphite",
    name: "Graphite",
    blurb: "The quiet instrument panel. No sound, no fuss.",
    words: graphiteWords,
    particles: 0,
  },
  {
    id: "tavern",
    name: "Tavern Ledger",
    blurb: "A quest log by candlelight: parchment, ink and brass. Catches clink like coin.",
    words: {
      tabs: { live: "The Pier", recorder: "Chronicle", settings: "Ledger" },
      start: "Set out to fish",
      stop: "Pack up the rod",
      catches: ["fish in the creel", "fish in the creel"],
      empty: "Set out, and the waters will show here",
      support: "Buy the barkeep a pint",
      cycle: {
        focus: ["Attend", "Awaiting the game's attention"],
        searching: ["Seek", "Seeking the bobber upon the water"],
        monitoring: ["Wait", "Waiting for a nibble"],
        reeling: ["Haul", "Hauling in the catch"],
        cooldown: ["Rest", "Resting a moment by the fire"],
        casting: ["Cast", "Casting the line"],
      },
    },
    sound: (ctx) => {
      tone(ctx, { type: "triangle", freq: 1568, len: 0.18, gain: 0.25 });
      tone(ctx, { type: "triangle", freq: 2093, at: 0.07, len: 0.3, gain: 0.2 });
      tone(ctx, { type: "triangle", freq: 2637, at: 0.16, len: 0.35, gain: 0.12 });
    },
    particles: 16,
  },
  {
    id: "arcane",
    name: "Arcane Grimoire",
    blurb: "Violet runes and a scrying glass that glows while it watches. Catches chime upward.",
    words: {
      tabs: { live: "Scrying", recorder: "Archive", settings: "Tome" },
      start: "Begin the ritual",
      stop: "Break the circle",
      catches: ["fish conjured", "fish conjured"],
      empty: "Begin the ritual to open the scrying glass",
      support: "Brew me a mana potion",
      cycle: {
        focus: ["Attune", "Attuning to the game window"],
        searching: ["Divine", "Divining where the bobber lies"],
        monitoring: ["Scry", "Scrying for a bite"],
        reeling: ["Summon", "Summoning the catch"],
        cooldown: ["Channel", "Channelling mana"],
        casting: ["Invoke", "Invoking the cast"],
      },
    },
    sound: (ctx) => {
      [523, 659, 784, 1047, 1319].forEach((freq, i) => tone(ctx, { type: "sine", freq, at: i * 0.07, len: 0.9, gain: 0.12 }));
    },
    particles: 22,
  },
  {
    id: "frost",
    name: "Frostwater",
    blurb: "Quiet glass and drifting snow. Calm, airy and light. Catches ring like a bell.",
    words: {
      tabs: { live: "Ice hole", recorder: "Memories", settings: "Camp" },
      start: "Cut the ice",
      stop: "Head home",
      catches: ["fish from the ice", "fish from the ice"],
      empty: "Cut the ice to look into the water",
      support: "Buy me a hot cocoa",
      cycle: {
        focus: ["Still", "Waiting for the game window to settle"],
        searching: ["Spot", "Spotting the bobber"],
        monitoring: ["Hush", "Hush, watching for a bite"],
        reeling: ["Pull", "Pulling it through the ice"],
        cooldown: ["Thaw", "Warming the hands"],
        casting: ["Drop", "Dropping the line"],
      },
    },
    sound: (ctx) => {
      tone(ctx, { type: "sine", freq: 2637, len: 1.6, gain: 0.14 });
      tone(ctx, { type: "sine", freq: 3951, len: 1.0, gain: 0.06 });
      tone(ctx, { type: "sine", freq: 1319, at: 0.02, len: 1.8, gain: 0.08 });
    },
    particles: 14,
  },
  {
    id: "fel",
    name: "Felfire Forge",
    blurb: "Black iron, green fire and hard edges. Loud, fast and the controls sit on the left.",
    words: {
      tabs: { live: "Hunt", recorder: "Records", settings: "Forge" },
      start: "Unleash",
      stop: "Quench",
      catches: ["fish claimed", "fish claimed"],
      empty: "Unleash to stoke the forge",
      support: "Stoke the maker's forge",
      cycle: {
        focus: ["Lock", "Locking onto the game window"],
        searching: ["Hunt", "Hunting the bobber"],
        monitoring: ["Stalk", "Stalking the bite"],
        reeling: ["Rip", "Ripping it from the water"],
        cooldown: ["Smoulder", "Smouldering"],
        casting: ["Hurl", "Hurling the line"],
      },
    },
    sound: (ctx) => {
      tone(ctx, { type: "sawtooth", freq: 160, to: 45, len: 0.5, gain: 0.3, filter: 900 });
      tone(ctx, { type: "square", freq: 90, to: 40, at: 0.04, len: 0.4, gain: 0.12, filter: 500 });
    },
    particles: 26,
  },
  {
    id: "tide",
    name: "Tidecaller",
    blurb: "Sun, surf and bubbles. Round, bright, bouncy, with the tabs down at your thumb.",
    words: {
      tabs: { live: "Beach", recorder: "Logbook", settings: "Tackle box" },
      start: "Cast off!",
      stop: "Back to shore",
      catches: ["fish hauled in", "fish hauled in"],
      empty: "Cast off to see the waves here",
      support: "Toss me some bait",
      cycle: {
        focus: ["Ahoy", "Waiting for the game window"],
        searching: ["Spot", "Looking for the bobber"],
        monitoring: ["Bob", "Bobbing along, waiting for a bite"],
        reeling: ["Reel!", "Reel it in!"],
        cooldown: ["Chill", "Catching a breather"],
        casting: ["Cast", "Casting out"],
      },
    },
    sound: (ctx) => {
      tone(ctx, { type: "sine", freq: 260, to: 820, len: 0.14, gain: 0.3 });
      tone(ctx, { type: "sine", freq: 340, to: 1100, at: 0.12, len: 0.12, gain: 0.22 });
      tone(ctx, { type: "sine", freq: 420, to: 1400, at: 0.22, len: 0.1, gain: 0.15 });
    },
    particles: 18,
  },
];

export const themeById = (id: string) => themes.find((t) => t.id === id) ?? themes[0];

/** Theme sounds are a per-viewer choice, on unless turned off. */
export const sounds = {
  on: () => storage.get("theme-sounds") !== "off",
  set: (on: boolean) => storage.set("theme-sounds", on ? "on" : "off"),
};

let audio: AudioContext | undefined;

/** playSound plays the theme's catch sound when sounds are on. */
export function playSound(theme: Theme): void {
  if (!theme.sound || !sounds.on()) return;
  audio ??= new AudioContext();
  void audio.resume();
  theme.sound(audio);
}

/** burst throws the theme's particles over el; themes.css shapes and moves them. */
export function burst(theme: Theme, el: HTMLElement): void {
  if (!theme.particles || matchMedia("(prefers-reduced-motion: reduce)").matches) return;
  for (let i = 0; i < theme.particles; i++) {
    const p = document.createElement("i");
    p.className = "particle";
    p.setAttribute("aria-hidden", "true");
    p.style.setProperty("--x", `${Math.random() * 100}%`);
    p.style.setProperty("--y", `${30 + Math.random() * 50}%`);
    p.style.setProperty("--drift", `${(Math.random() - 0.5) * 160}px`);
    p.style.setProperty("--spin", `${(Math.random() - 0.5) * 720}deg`);
    p.style.setProperty("--size", `${0.7 + Math.random() * 0.9}`);
    p.style.animationDelay = `${Math.random() * 0.35}s`;
    p.addEventListener("animationend", () => p.remove());
    el.append(p);
  }
}
