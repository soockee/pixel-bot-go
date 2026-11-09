---
title: "States and events"
summary: "The fishing state machine: every state, every event and what moves it on."
order: 5
---

## States

| State         | Meaning                 | Exit Condition                              |
| ------------- | ----------------------- | ------------------------------------------- |
| halt          | Idle                    | Enable capture → waiting_focus              |
| waiting_focus | Await game window focus | Focus → searching                           |
| searching     | Scan template scales    | Found → monitoring; timeout/force → casting |
| monitoring    | Watch for a bite        | Bite → reeling; lost or timeout → casting   |
| reeling       | Move, click after 300 ms | Click → cooldown                           |
| cooldown      | Delay before next cast  | Timer → casting                             |
| casting       | Send cast key           | Immediate internal → searching              |

## Events

| Event                    | Source               | Effect                          |
| ------------------------ | -------------------- | ------------------------------- |
| EventAwaitFocus          | User enables capture | halt → waiting_focus            |
| EventFocusAcquired       | Focus watcher        | waiting_focus → searching       |
| EventTargetAcquiredAt    | Engine               | searching → monitoring          |
| EventTargetLost          | Tests                | monitoring → casting            |
| EventFishBite            | Engine               | monitoring → reeling            |
| ForceCast                | Tests                | searching, monitoring, cooldown → casting |
| Deadline                 | One state timer      | searching, monitoring → casting; reeling → click → cooldown; cooldown → casting |
| EventFocusLost           | Focus watcher        | any active state → waiting_focus |
| EventHalt                | Stop button, input guard  | any → halt                 |
