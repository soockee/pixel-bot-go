# Pixel Bot

Pixel Bot is a small Windows app that watches part of your screen and tries to spot a specific image (for example a fishing bobber in a game). When it finds the image it can move your mouse to that spot. You control everything from a simple window—no command line knowledge required.

## What You See
When you start the program a window appears with:
- A timer and status text (top)
- A panel of settings you can change
- A preview area that shows the latest captured image
- Buttons to start/stop capture and exit

## Getting Started
1. Install Go (if you just want to run the pre-built binary you can skip building, but building is easy): https://go.dev/dl/
2. Open PowerShell in the project folder.
3. (Recommended) Run PowerShell as Administrator so mouse movement works reliably in games.

Build and run:
```powershell
go mod tidy
go build -o pixel-bot-go.exe
./pixel-bot-go.exe
```

## Using The App
1. Press the "Toggle Capture" button to start watching the screen.
2. The preview will update and the status will say whether the target was found.
3. If found, the mouse moves to the detected location automatically.
4. To change how it searches, stop capture first (press the button again). Then edit the numbers/text in the settings panel and click "Apply Changes".

If you apply changes while capture is on, the app will ignore them and remind you to pause capture first.

## Settings for Dummies
- Min / Max Scale: How small or large the target might appear. Leave default unless you know the object changes size a lot.
- Scale Step: How finely it searches between min and max. Smaller = slower but can be a little more accurate.
- Threshold: How sure it must be before saying "found" (higher means stricter).
- Stride: How big a jump it makes when scanning. Lower = slower but more precise.
- Refine: Extra fine check near the best spot (usually keep ON = true).
- Use RGB: Use color for matching (usually keep ON = true for better results).
- Stop On Score: If it gets at least this score it stops early to be faster.
- Return Best Even: If true it still tells you the best spot even if it wasn't confident enough.

You don't have to understand the math behind these—defaults are chosen to work reasonably well.

## Logs
The program writes simple JSON lines telling you what settings it started with and every time you apply new ones. You can ignore them unless you're curious. They look like:
```jsonc
{"level":"INFO","msg":"initial config", ...}
{"level":"INFO","msg":"config applied", ...}
```

## Curious About The Internals?
Want to peek under the hood? See:
* `docs/DETECTION.md` – how image matching works.
* `docs/STATE_MACHINE.md` – the fishing steps and bite detection logic.
Totally optional.

## Optional Command Line Flags
You can still start the app with flags or a JSON file to pre-fill the settings (for advanced users). Most people can ignore this and just use the window.

## Safety Notice
Automating input in games can break their rules. Use at your own risk and only in places where it's allowed.

## Troubleshooting
- Mouse didn't move: Try running as Administrator.
- Detection seems slow: Increase Stride or Threshold slightly; keep capture off when applying changes.
- Too many false matches: Raise Threshold or turn on Use RGB.

Enjoy experimenting!


## Selection Area (Optional)
You can define a rectangle so the app watches only part of the screen (helpful to reduce noise). Use the "Selection Grid" button to set or clear it. This improves speed and focus; leave it unset for full-screen watching.

## Performance & Memory Notes
High-resolution screen grabs allocate large RGBA buffers (≈8 MB for 1080p). To reduce steady-state heap growth the capture loop now copies each raw screenshot into a reusable buffer (pool). After the UI / detection logic is done with a frame it is recycled. This lowers retained heap size under slow consumers. Future improvement: capture directly into the reusable buffer using a Windows API (BitBlt + GetDIBits) to eliminate the temporary allocation from the screenshot library.
If you profile (`go test -bench . -benchmem`) you should see fewer long-lived allocations, though per-frame allocation still occurs until the direct capture path is added.

