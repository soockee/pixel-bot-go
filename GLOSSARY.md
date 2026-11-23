# Glossary

**Board**: The paired ESP32 the player connects over USB. It is the only way the app gives the game a key press, a click or a pointer move.
_Avoid_: Keyboard (it does more than keys), device

**Input**: Everything the app does to the game: key presses, holds, clicks, pointer moves and camera drags. All of it goes to the Board, or only to the log in a dry run; the app never creates input in software.
_Avoid_: Gateway, action (an action is a step of the cycle, not the input it sends)

**Pause**: A run waiting for the game to be in front again. It starts whenever the game loses focus, however close to an input, and the run continues on its own once the game is back in front.
_Avoid_: Halt (a halt ends the run), waiting focus

**Stop**: Ending a run, by the player's Stop button or by the run itself when it cannot go on. A stopped run does not continue on its own.
_Avoid_: Pause
