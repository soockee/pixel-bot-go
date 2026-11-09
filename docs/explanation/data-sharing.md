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

Recording is off unless you switch on **Share recordings** in Settings, start a recording, and later confirm **Upload reviewed copy**. See [Record and share a session](../how-to/share-recordings.md). A recording holds:

- **pictures of the game window**, taken while the game is in front. They can show names and chat;
- your **mouse**: where it is and what you click, while it is over the game;
- your **keys**, only if you switch on **Record keys**, and only on your PC: keys are never uploaded;
- the **game's sound** only, not music, voice chat or other programs.

Reviewing an upload sends nothing. You choose whether pictures and sound go along. The recording itself stays on your PC.

## What it is used for

Recordings are real examples of bobbers and bites in different places, light and weather. They are used to test and improve how Go Fish finds the bobber and spots a bite, so it works for more people in more places. They are not sold or given to anyone.

## Where it goes and who can see it

- Uploads are stored in Go Fish's private storage on Google Cloud in Frankfurt, Germany, in a folder named after your email.
- Only the project owner can open them. Other players can't see your recordings, and you can't see theirs.
- A recording can't be changed after upload.
- Recordings are kept until the owner deletes them, and can't be deleted in the first 30 days. To have yours removed, ask the owner on [GitHub](https://github.com/soockee/pixel-bot-go/issues).

## Your account and role

Signing in tells Go Fish who you are so the service can check whether you may share recordings. Fishing does not require an account. Go Fish asks Google only for your email address. Your role is kept on Go Fish's server; your PC keeps only an encrypted sign-in so you don't have to sign in every time. **Sign out** deletes it.
