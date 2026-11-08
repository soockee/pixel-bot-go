---
title: "Accounts and data sharing"
summary: "Go Fish sends no gameplay data unless you choose to share a recording. Here is what it does send, what a recording holds, where it goes and what it is for."
order: 2
---

## What Go Fish sends while you just fish

Nothing about your fishing. The pictures it looks at, the casts and the catches stay on your PC. It only talks to the internet for three things:

| To | When | What |
| --- | --- | --- |
| Google | When you sign in, and now and then to keep you signed in | Google confirms your email address to Go Fish. Go Fish never sees your password. |
| Go Fish's server | At start, when you are signed in | Your sign-in, to look up your role. |
| GitHub | At start and every hour | A request for the newest version number. Nothing about you. |

## What a shared recording holds

Recording is off unless you switch on **Share recordings** in Settings, start a recording, and later click **Upload**. See [Record and share a session](../how-to/share-recordings.md). A recording holds:

- pictures of the **game window only**, about ten a second, never the rest of your screen;
- your **mouse**: where it is and what you click, while it is over the game;
- your **keys** while the game is in front, including what you type in chat, unless you switch off **Record keys**;
- the **game's sound** only, not music, voice chat or other programs.

It holds no passwords, no files from your PC, and nothing from other windows.

## What it is used for

Recordings are real examples of bobbers and bites in different places, light and weather. They are used to test and improve how Go Fish finds the bobber and spots a bite, so it works for more people in more places. They are not sold or given to anyone.

## Where it goes and who can see it

- Uploads are stored in Go Fish's private storage on Google Cloud in Frankfurt, Germany, in a folder named after your email.
- Only the project owner can open them. Other players can't see your recordings, and you can't see theirs.
- A recording can't be changed after upload.
- Recordings are kept until the owner deletes them, and can't be deleted in the first 30 days. To have yours removed, ask the owner on [GitHub](https://github.com/soockee/pixel-bot-go/issues).

## Your account and role

Signing in tells Go Fish who you are, so the owner can decide who may fish and who may share recordings. Go Fish asks Google only for your email address. Your role is kept on Go Fish's server; your PC keeps only an encrypted sign-in so you don't have to sign in every time. **Sign out** deletes it.
