// The frontend's whole contract with Go: typed service calls generated from
// ui/desktop, and the state event. Nothing else talks to the backend.
import { Events } from "@wailsio/runtime";
import { Service } from "../bindings/github.com/soocke/pixel-bot-go/ui/desktop";
import type { State } from "../bindings/github.com/soocke/pixel-bot-go/ui/desktop";
import * as AppEvents from "../bindings/events";

export { Service };
export type { Recording, Settings, SettingsForm, State } from "../bindings/github.com/soocke/pixel-bot-go/ui/desktop";
export type { EventPage } from "../bindings/github.com/soocke/pixel-bot-go/internal/recorder";

/** onState calls fn with the current state and with every change. */
export function onState(fn: (state: State) => void): void {
  Events.On(AppEvents.State, (event) => fn(event.data));
  void Service.State().then(fn);
}

/** framesUrl is the newest preview image of one stream. */
export const framesUrl = (name: "capture" | "roi" | "scene", version: number) => `/frames/${name}?v=${version}`;

/** recordingFrameUrl is the newest frame of a recording. */
export const recordingFrameUrl = (name: string) => `/recordings/${encodeURIComponent(name)}?t=${Date.now()}`;
