// The Overlay section of Settings: a button that lets the person drag the
// in-game pill to where they want it. Go checks it can (no run, the game is
// open) and saves the place when the person is done.
import { Service, type State } from "./api";
import { $ } from "./dom";

export function setupMoveOverlay(): (state: State) => void {
  const button = $<HTMLButtonElement>("move-overlay");
  const detail = $("overlay-detail");
  let moving = false;

  // Go shows why it could not start in the status bar.
  button.addEventListener("click", () => void Service.MoveOverlay(!moving).catch(() => undefined));

  return (s) => {
    moving = s.overlayMove;
    button.textContent = moving ? "Done" : "Move overlay";
    button.classList.toggle("primary", moving);
    button.disabled = s.running && !moving;
    detail.textContent = moving
      ? "Drag the pill in the game to where you want it, then click Done."
      : s.running
        ? "Stop the run to move the overlay."
        : "Shown over the game while it fishes";
  };
}
