---
title: "Update the app"
summary: "Go Fish finds new releases by itself; one click downloads, verifies and installs them."
order: 3
---

At start and then every hour, a release build asks GitHub for the latest release of `soockee/pixel-bot-go`. When it is newer, the top bar shows **Update to vX.Y.Z**. **Settings > Updates** shows the installed and latest version, when the app last checked, a **Check for updates** button and **What's new**, which opens the release notes.

1. Stop the run, the recording and any upload; the button stays disabled until then.
2. Click **Update to vX.Y.Z**. The status bar shows the download; the app checks the build against the release's `CHECKSUMS.txt`, refuses it on a mismatch, then replaces itself and restarts with the same flags.

The replaced build stays beside the exe as `pixel-bot-go.exe.old` until the next start removes it. Local builds (`go build`) report version `dev` and never check. To try a version, build with `-ldflags "-X github.com/soocke/pixel-bot-go/internal/update.Version=v1.0.0"`.
