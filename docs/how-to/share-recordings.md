---
title: "Record and share a session"
summary: "Record yourself fishing by hand and send it in, to help Go Fish get better at spotting bobbers and bites. Off unless you turn it on."
order: 5
---

Recordings help make Go Fish's bobber and bite detection better. Sharing is optional and off by default: nothing is recorded or sent unless you do the steps below. Read [Accounts and data sharing](../explanation/data-sharing.md) first, so you know what a recording holds and where it goes.

## Turn on the Recorder

The Recorder is hidden by default.

1. Close Go Fish.
2. In the folder with the Go Fish exe, open `pixle_bot_config.json` in Notepad. If there is no such file yet, create it.
3. Change `"debug": false` to `"debug": true` and save. In a new file, write just `{"debug": true}`.
4. Start Go Fish. A **Recorder** tab appears.

## Record

1. On the **Fishing** tab, check that the game window is selected.
2. Open the **Recorder** tab and click **Record**.
3. Switch to the game and fish by hand as usual: cast, wait, click the bobber when it splashes. A few minutes in one spot is plenty; different places and times of day help most.
4. Press **F10**, or click **Stop recording**.

While it records, Go Fish saves a picture of the game window about ten times a second, your mouse, your keys while the game is in front, and the game's sound. Keys include what you type in chat. If you'd rather not record keys, switch off **Record keys** under **Settings → Recording** first.

Recordings stay on your PC, in the `datasets` folder next to Go Fish, until you share them.

## Share

Sharing needs a role that may upload; ask the owner.

1. On the **Recorder** tab, pick the recording.
2. Click **Upload**. The status bar counts the files.

If the upload stops halfway, click **Upload** again; files already sent are skipped. Once sent, a recording can't be changed. To have it removed, ask the owner.
