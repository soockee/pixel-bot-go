import { createElement, type IconNode } from "lucide";

/** $ returns the element with id, which the page always has. */
export function $<T extends HTMLElement = HTMLElement>(id: string): T {
  const el = document.getElementById(id);
  if (!el) throw new Error(`missing #${id}`);
  return el as T;
}

/** setIcon replaces the content of el with one line icon. */
export function setIcon(el: Element, icon: IconNode): void {
  el.replaceChildren(createElement(icon, { width: 16, height: 16, "stroke-width": 1.5 }));
}

/** h creates an element with text. */
export function h<K extends keyof HTMLElementTagNameMap>(tag: K, text = "", className = ""): HTMLElementTagNameMap[K] {
  const el = document.createElement(tag);
  el.textContent = text;
  if (className) el.className = className;
  return el;
}

/** clock formats seconds as m:ss, or h:mm:ss from an hour on. */
export function clock(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds));
  const hh = Math.floor(s / 3600);
  const mm = Math.floor((s % 3600) / 60);
  const ss = String(s % 60).padStart(2, "0");
  return hh ? `${hh}:${String(mm).padStart(2, "0")}:${ss}` : `${mm}:${ss}`;
}

/** css reads a theme token from :root. */
export const css = (name: string) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();

/** storage reads and writes one per-viewer convenience; it may be unavailable. */
export const storage = {
  get(key: string): string | null {
    try { return localStorage.getItem(key); } catch { return null; }
  },
  set(key: string, value: string): void {
    try { localStorage.setItem(key, value); } catch { /* not kept */ }
  },
};
