import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import axe from "axe-core";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SIDEBAR_STORAGE_KEY } from "@/config";
import { applyLanguage } from "@/i18n";
import { answerFetch, renderApp } from "@/test/render";

const META = { product: "bearing", build: { version: "0.1.0+dev" }, apiVersion: "1", capabilities: [] };

afterEach(() => {
  // Unmounted first: a language reset with the app still mounted re-renders it outside act.
  cleanup();
  vi.restoreAllMocks();
  applyLanguage("");
  localStorage.clear();
});

describe("the shell", () => {
  it("leads to every place and marks the current one", async () => {
    answerFetch(META);
    const user = userEvent.setup();
    renderApp("/");
    const nav = await screen.findByRole("navigation", { name: "Main" });
    expect(within(nav).getByRole("link", { name: "Overview" })).toHaveAttribute("aria-current", "page");
    await user.click(within(nav).getByRole("link", { name: "People" }));
    expect(await screen.findByRole("heading", { level: 1, name: "People" })).toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "People" })).toHaveAttribute("aria-current", "page");
    expect(within(nav).getByRole("link", { name: "Overview" })).not.toHaveAttribute("aria-current");
  });

  it("folds the sidebar to a rail and remembers it", async () => {
    answerFetch(META);
    const user = userEvent.setup();
    renderApp("/");
    await user.click(await screen.findByRole("button", { name: "Fold the sidebar" }));
    expect(screen.getByRole("button", { name: "Unfold the sidebar" })).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("Bearing")).not.toBeInTheDocument();
    expect(localStorage.getItem(SIDEBAR_STORAGE_KEY)).toBe("folded");
  });

  it("switches the language from the account menu", async () => {
    answerFetch(META);
    const user = userEvent.setup();
    renderApp("/");
    await user.click(await screen.findByRole("button", { name: "Account and settings" }));
    await user.click(screen.getByRole("radio", { name: "Deutsch" }));
    expect(await screen.findByRole("link", { name: "Personen" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Übersicht" })).toBeInTheDocument();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("radio", { name: "Deutsch" })).not.toBeInTheDocument();
  });

  it("answers an unknown address with a way back", async () => {
    answerFetch(META);
    renderApp("/nowhere");
    expect(await screen.findByRole("heading", { level: 1, name: "Nothing here" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Back to the overview" })).toHaveAttribute("href", "/");
  });

  it("has no accessibility violations", async () => {
    answerFetch(META);
    const { container } = renderApp("/");
    await screen.findByText("API 0.1.0+dev");
    const results = await axe.run(container, { rules: { region: { enabled: false } } });
    expect(results.violations.map((v) => `${v.id}: ${v.help}`)).toEqual([]);
  });
});

describe("the overview", () => {
  it("names the API's build", async () => {
    const fetch = answerFetch(META);
    renderApp("/");
    expect(await screen.findByText("API 0.1.0+dev")).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledWith("/api/v1/meta", expect.anything());
  });

  it("says when the API does not answer", async () => {
    answerFetch({ error: { code: "unavailable", message: "down" } }, 503);
    renderApp("/");
    await waitFor(() => expect(screen.getByTestId("api-status")).toHaveTextContent("The API does not answer."));
  });
});
