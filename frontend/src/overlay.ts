// Overlay: lies over the game during a run and shows how it goes, so the
// player never has to leave the game. Go places it and it takes no clicks,
// except while the person places it: then it is only the pill, to be dragged.
import { onState } from "./api";
import { $ } from "./dom";
import { themeById } from "./themes";

const step = $("hud-step");
const catches = $("hud-catches");
const status = $("hud-status");
const box = $("box");

onState((s) => {
  const root = document.documentElement.style;
  root.setProperty("--x", String(s.overlayX));
  root.setProperty("--y", String(s.overlayY));
  document.body.classList.toggle("moving", s.overlayMove);
  document.body.classList.toggle("low", s.overlayY > 0.5);
  document.body.hidden = !s.running && !s.overlayMove;
  if (s.overlayMove) {
    step.textContent = "Drag to place the overlay";
    status.hidden = box.hidden = true;
    return;
  }
  if (!s.running) return;
  const words = themeById(s.theme).words;
  step.textContent = words.cycle[s.fsm]?.[1] ?? "Starting";
  catches.textContent = `${s.reels} ${words.catches[s.reels === 1 ? 0 : 1]}`;
  status.hidden = s.status === "" || s.status === "Ready";
  status.textContent = s.status;
  // roiBox is in screen pixels on the game's client area, which the overlay
  // covers. The outline goes around it, outside what the bite detector sees,
  // because the bot's screen capture includes this window.
  const watching = s.fsm === "monitoring" && s.roiBox.w > 0;
  box.hidden = !watching;
  if (watching) {
    const gap = 4;
    const px = (v: number) => `${v / devicePixelRatio}px`;
    Object.assign(box.style, {
      left: `calc(${px(s.roiBox.x)} - ${gap}px)`,
      top: `calc(${px(s.roiBox.y)} - ${gap}px)`,
      width: `calc(${px(s.roiBox.w)} + ${2 * gap}px)`,
      height: `calc(${px(s.roiBox.h)} + ${2 * gap}px)`,
    });
  }
});
