import { SIDEBAR_RAIL_BELOW_PX, SIDEBAR_STORAGE_KEY } from "@/config";

/** Whether the sidebar starts folded to its rail: as this browser remembers it, else folded on a narrow screen. */
export function readFolded(viewportWidth: number = window.innerWidth): boolean {
  try {
    const stored = localStorage.getItem(SIDEBAR_STORAGE_KEY);
    if (stored === "folded" || stored === "open") return stored === "folded";
  } catch {
    // A browser that keeps nothing falls back to the screen's width.
  }
  return viewportWidth < SIDEBAR_RAIL_BELOW_PX;
}

/** Remembers whether the sidebar is folded; a browser that keeps nothing still folds it for this page. */
export function writeFolded(folded: boolean): void {
  try {
    localStorage.setItem(SIDEBAR_STORAGE_KEY, folded ? "folded" : "open");
  } catch {
    // Nothing to keep it in.
  }
}
