<a name="top"></a>

# Pixel Bot (Simple Guide)

Pixel Bot is a small Windows app that looks at a part of your screen, tries to spot a little picture (for example a fishing bobber in World of Warcraft), and when it sees it, can move your mouse there and press a key for you. Think of it as an on‑screen helper that watches and reacts while you relax.

> IMPORTANT: Using automation in online games can break their rules. Only use this where you are sure it is allowed. You take full responsibility.

---

## 1. What It Can Do (Plain English)
* Watch the screen (or just a chosen rectangle) for a tiny object/pattern.
* Automatically aim the mouse at it when it appears.
* Run a simple fishing loop: cast → watch bobber → detect bite → reel in → wait → cast again.
* Optional dark mode so it is easy on your eyes.
* Lets you pause, change settings, and resume without fuss.

## Demo

https://github.com/user-attachments/assets/002c679b-130e-422f-99b9-4503c648b8f3


---

## 2. Quick Start (Fastest Path)
1. Download / build the app (see "Build Yourself" below if you want).
2. Run `pixel-bot-go.exe` (preferably right‑click → Run as Administrator so mouse / key presses work in the game).
3. In your game, get your character ready to fish (clear view of the bobber area).
4. In Pixel Bot click the button to start capture / fishing.
5. Watch the status text. It will say things like Searching, Monitoring, Cooldown.
6. To stop, press the same button again.

That’s it. If it “misses” or is too trigger‑happy, adjust a couple of settings (explained below) and try again.

---

## 3. The Window You’ll See
Top area: current state (e.g. Searching, Monitoring, Cooldown) plus two timers:
* Session – elapsed time of the current active capture periods (sums across toggles until you exit or restart).
* Total – cumulative time across all capture periods since launch (includes the ongoing session while active).
They update live only while capture is enabled; when you pause they freeze until you resume.
Middle left: settings panel (numbers, checkboxes, buttons to apply changes, selection area tool).
Middle right: live preview image of what it is looking at.
Bottom: buttons (Start / Stop, maybe Exit, maybe Selection Grid, etc.).

You can resize the window. Changes only apply when capture is paused (to keep things stable).

---

## 4. Easy Configuration Guide (For Non‑Tech Users)
You’ll see several fields. You can leave them as they are. If you want to tweak:

Template Search (finding the bobber / object)
* Min Scale & Max Scale: Smallest and largest size the bot will look for. If the object never really changes size, leave them alone.
* Scale Step: How fine the size steps are between Min and Max. Smaller step = more tries = slower but thorough. Bigger step = faster.
* Threshold: How “sure” the bot must feel before saying “Found it!”. If it falsely triggers, raise this number a little. If it never finds it, lower a bit.
* Stride: How many pixels it skips while scanning. Bigger = faster, but can be a little less precise; smaller = slower but accurate.
* Refine (checkbox): After a quick scan, do a tiny careful scan around the best spot. Usually keep ON.
* Use RGB (checkbox): Use full color. ON is usually better in colorful games. Turn OFF only if you want a tiny speed boost.
* Stop On Score: If the match score reaches this number, it stops early to save time. Leave the default.
* Return Best Even: If ON, it still tells you where it “thinks” the object is even if below Threshold (mostly for advanced tweaking / curiosity).

Fishing Loop Settings
* reel_key: The key it will press to reel in (example: F3). Make sure this matches your in‑game keybinding for reeling / interact.
* cooldown_seconds: Time to wait after reeling in before trying to cast again (lets loot animation finish).
* max_cast_duration_seconds: Safety timer. If nothing happens for this many seconds after a cast, it gives up and casts again.
* roi_size_px: Size of the little square it watches once it locks onto the bobber (bigger = a bit slower, smaller = might miss motion).

Visual / Comfort
* dark_mode: Switch the app’s look between light and dark styles.

Selection Area (Optional)
* selection_x / selection_y / selection_w / selection_h: These numbers define the rectangle of the screen to watch. You normally set / clear this using the Selection Grid button instead of typing numbers. Leaving it unset (or the tool cleared) means “watch entire screen”. Limiting the area can speed things up and reduce mistakes.

Advanced / Internal
* debug: When true, writes extra information to logs. Normal users keep this false.
* analysis_scale: Internal scaling for some calculations. Leave at 1.

If something looks scary: ignore it—defaults are sensible.

---

## 5. Files The App Creates (Don’t Panic)
After running you may see a couple of new files in the same folder:

* `pixle_bot_config.json` – Your saved settings so next time it remembers them. You can open it in Notepad and change values while the app is CLOSED. If you break the file (bad commas etc.) the app may reset or complain; if that happens just delete it and it will regenerate with defaults.
* `pixel_bot_logs.json` – A log file. Each line is a tiny piece of text in JSON that says what happened (e.g. settings applied, dark mode changed, detection events). You can delete it; a new one will be created automatically. Useful if you ask for help and someone wants to see what happened.

You can safely move or delete both; they’ll come back with default content next run.

---

## 6. Building It Yourself (Optional)
If you prefer or need to build:

```powershell
go mod tidy
go build -o pixel-bot-go.exe
./pixel-bot-go.exe
```

Run PowerShell as Administrator if mouse movement or key presses do nothing in the game window.

For profiling and deeper diagnostics (memory leak, CPU hotspots), see `docs/DEBUG.md`.

---

## 7. Day‑To‑Day Use Tips
* Start the bot only when the fishing bobber is visible area (no big UI windows covering it).
* Keep the game window focused so the key press works.
* If detection gets slower over time, pause, apply settings again, or narrow the Selection Area.
* Don’t run other overlay tools that heavily flash or animate over the bobber region.

---

## 8. Common Problems & Simple Fixes
| Problem                             | Try This                                                                                  |
| ----------------------------------- | ----------------------------------------------------------------------------------------- |
| Mouse not moving / key not pressed  | Run as Administrator; confirm correct `reel_key`.                                         |
| Never finds bobber                  | Lower Threshold a little; ensure Min/Max Scale bracket the actual size; enable Use RGB.   |
| Triggers on wrong things            | Raise Threshold; set a Selection Area just around the water; keep Use RGB ON.             |
| Too slow / laggy                    | Increase Stride a bit; make Scale Step larger; restrict Selection Area.                   |
| Window text hard to read            | Toggle dark_mode or resize the window.                                                    |
| Wants to fish forever after no bite | Check `max_cast_duration_seconds` isn’t too large; ensure the bobber is actually visible. |

---

## 9. Safety & Fair Play
This tool simulates input. Some games forbid that. Use only where allowed (private realms, sandbox, personal experimentation). If unsure—don’t use it. Close the program before chatting with support staff of any game: better safe than sorry.

---

## 10. Want to Learn More (Optional Reading)
If curiosity strikes:
* `docs/DETECTION.md` – How it recognizes the bobber image.
* `docs/STATE_MACHINE.md` – How the fishing loop logic flows inside.

Not needed for normal use.

---

## 11. Frequently Asked Questions (FAQ)
**Q: Do I have to change the settings?**  
No. Try defaults first.

**Q: Can I break my game account?**  
If the game forbids automation, possibly. Use only where safe.

**Q: Why doesn’t it always click instantly?**  
It waits until it’s fairly sure. Lower Threshold or reduce Stride to speed up (may increase mistakes).

**Q: Can I move the game window?**  
Yes, but if the bobber moves outside the Selection Area you set, detection stops until it’s inside again.

**Q: It created new JSON files—virus?**  
No—those are just configuration and logs (see section 5).

**Q: How do I reset everything?**  
Close the app, delete `pixle_bot_config.json` and (optionally) `pixel_bot_logs.json`, then open the app again.

---

## 12. License / Credits
Licensed under the MIT License. See [`LICENSE`](LICENSE) for full text.

Copyright (c) 2025 Simon Stockhause.

If you use or fork this project, please keep a visible credit such as:
"Pixel Bot by Simon Stockhause" in your README or about dialog where feasible.

Provided as‑is with no warranty. Use responsibly.

---

## 13. Short Technical Corner (Skip if you like)
The program repeatedly takes screenshots, searches for a template at different sizes, and scores matches. When the match is good enough it moves the cursor. A small state machine handles the fishing cycle (search → watch → reel → wait). That’s all.

---

[Back to Top](#top)



