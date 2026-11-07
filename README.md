<a name="top"></a>

# Visual Response Lab

Visual Response Lab is a Windows application for studying visual event detection and response timing. The local simulator renders a scripted fishing scene with a visual target and timed splash events. The application captures a selected region, locates the target, and reacts to those events. Dry-run mode records intended actions in the log.

---

## Quick Start
1. Run `go run ./cmd/fakegame` in one terminal to start the simulator.
2. Run `go run . -dry-run` in another terminal to start the application without Administrator privileges.
3. Select the simulator window and its capture region, then press Start.
4. Focus the simulator window and observe the state transitions.
5. Press Stop to end the run. Compare the simulator event timestamps with the intended actions in `pixel_bot_logs.json`.

Docs (Diataxis):
* [Tutorials](docs/TUTORIALS.md) – first simulator run and guided optimizations.
* [How-To Guides](docs/HOW_TO.md) – profiling, tuning, troubleshooting steps.
* [Reference](docs/REFERENCE.md) – algorithms, config parameters, state/event tables, FAQ.
* [Explanation](docs/EXPLANATION.md) – mental models, rationale, architecture concepts.

Work plan: [Architecture Review](docs/ARCHITECTURE_REVIEW.md) – findings and tasks for the foundation work.

---

MIT License in `LICENSE`.



