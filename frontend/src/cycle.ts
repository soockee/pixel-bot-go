// The fishing cycle in order, as fishing.FishingState names, with the short
// name the track shows and the caption the stage and the overlay show.
export const cycle: [state: string, step: string, caption: string][] = [
  ["focus", "Focus", "Waiting for the game window to have focus"],
  ["searching", "Search", "Looking for the bobber"],
  ["monitoring", "Watch", "Watching for a bite"],
  ["reeling", "Reel", "Reeling in"],
  ["cooldown", "Pause", "Pausing before the next cast"],
  ["casting", "Cast", "Casting"],
];

/** caption is what the bot is doing in state, or "" outside the cycle. */
export const caption = (state: string) => cycle.find(([s]) => s === state)?.[2] ?? "";

/** catches labels the catch count. */
export const catches = (n: number) => (n === 1 ? "catch this run" : "catches this run");
