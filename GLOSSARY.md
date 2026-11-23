# Glossary

**Board**: The paired ESP32 the player connects over USB. It is the only way the app gives the game a key press, a click or a pointer move.
_Avoid_: Keyboard (it does more than keys), device

**Input**: Everything the app does to the game: key presses, holds, clicks, pointer moves and camera drags. All of it goes to the Board, or only to the log in a dry run; the app never creates input in software.
_Avoid_: Gateway, action (an action is a step of the cycle, not the input it sends)
