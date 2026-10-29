import { afterEach, describe, expect, it } from "vitest";
import { SIDEBAR_STORAGE_KEY } from "@/config";
import { readFolded, writeFolded } from "./sidebar";

afterEach(() => localStorage.clear());

describe("the sidebar's fold", () => {
  it("starts as a rail on a narrow screen and open on a wide one", () => {
    expect(readFolded(800)).toBe(true);
    expect(readFolded(1280)).toBe(false);
  });

  it("keeps a person's choice over the screen's width", () => {
    writeFolded(false);
    expect(readFolded(800)).toBe(false);
    writeFolded(true);
    expect(readFolded(1280)).toBe(true);
    expect(localStorage.getItem(SIDEBAR_STORAGE_KEY)).toBe("folded");
  });
});
