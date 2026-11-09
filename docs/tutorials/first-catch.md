---
title: "Your first catch"
summary: "Install Go Fish and the Bobber Marker addon, sign in, set your Fishing key and watch it land a fish. About ten minutes."
order: 1
---

By the end of this page Go Fish will have cast, waited for a bite and reeled in a fish for you. You need World of Warcraft: Forever with the Fishing skill and a fishing pole, and a Windows PC.

## 1. Get the app

Download `pixel-bot-go-windows-amd64.exe` from the [latest release](https://github.com/soockee/pixel-bot-go/releases/latest) and put it in a folder of its own, for example `Documents\Go Fish`. Go Fish keeps its settings next to it.

## 2. Start Go Fish

Start World of Warcraft: Forever first. Then right-click the Go Fish exe and choose **Run as administrator**. Windows only lets an app press keys and click in a game, and write into the game's folder, when it runs as administrator.

## 3. Add the addon to the game

Go Fish finds your bobber by the pink brackets the **Bobber Marker** addon draws around it. The **Fishing** tab shows a **Get ready** checklist of what is still missing, with a button for each. Click **Install** there, or under **Addon** in **Settings**, then type `/reload` in the game. The checklist goes away when everything is in place. More in [Install the Bobber Marker addon](../how-to/install-addon.md).

## 4. Sign in

Click **Sign in with Google** in the top bar, or in the checklist. Your browser opens; pick your account and return to Go Fish. The top bar now shows your email and your role. If it says *no role*, ask the owner to give your account one; see [Sign in](../how-to/sign-in.md).

## 5. Choose your Fishing key

Open **Settings**. Under **Keys**, click **Cast key** and press the key you want Go Fish to cast with, for example `3` or `F3`. It saves at once. You don't set anything up in the game: the addon makes a **Fishing** macro and binds this key to it when you install or update the addon and `/reload`. If you change the key later, click **Update** under **Addon** and `/reload` again.

## 6. Fish

1. In the game, stand at the water and face it.
2. In Go Fish, open the **Fishing** tab. **World of Warcraft** is picked as the game window.
3. Click **Start fishing** and switch to the game.

Go Fish casts, finds the pink brackets around your bobber and watches the box. When the fish bites and the water splashes, it clicks the bobber, waits a moment and casts again. The Fishing tab shows each step and counts your catches.

## 7. Stop

Press **F10** at any time, wherever you are, or click **Stop** in Go Fish. It also stops by itself after an hour, or after five casts in a row without a bite. You can change both under [Settings](../reference/settings.md).

That's it. Next, [change keys and limits](../how-to/settings.md) to suit you, or read [how Go Fish works](../explanation/how-it-works.md).
