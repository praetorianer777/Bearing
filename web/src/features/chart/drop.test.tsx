import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { fakeAPI } from "@/test/render";
import type { Card } from "./graph";
import { DropChange } from "./OrgChart";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const card = (id: string, title: string, unitId: string, unitName: string, head = false): Card => ({
  id,
  title,
  unitId,
  unitName,
  head,
  seats: 1,
  holders: [],
  vacant: true,
  collapsible: false,
  collapsed: false,
  hidden: 0,
});

function dropped(source: Card, target: Card) {
  const writes: { path: string; body: unknown }[] = [];
  fakeAPI({
    "PUT /positions/tester/reports-to": (body) => {
      writes.push({ path: "reports-to", body });
      return { body: { position: {} } };
    },
    "POST /units/bench/move": (body) => {
      writes.push({ path: "move", body });
      return { body: { unit: {} } };
    },
    "POST /units/lab/move": { status: 409, body: { error: { code: "cycle", message: "A unit cannot sit inside itself." } } },
  });
  const done = vi.fn();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <DropChange source={source} target={target} onDone={done} />
    </QueryClientProvider>,
  );
  return { writes, done };
}

describe("a card dropped onto another", () => {
  it("proposes reporting there from a day, and makes it on confirming", async () => {
    const { writes, done } = dropped(card("tester", "Bench tester", "bench", "Bench"), card("lead", "Bench lead", "bench", "Bench", true));
    expect(screen.getByRole("region", { name: "Change the organisation" })).toBeVisible();
    expect(screen.getByLabelText("Bench tester reports to Bench lead")).toBeChecked();
    expect(screen.queryByLabelText(/moves into/)).toBeNull();
    fireEvent.change(screen.getByLabelText("From"), { target: { value: "2026-12-01" } });
    await userEvent.click(screen.getByRole("button", { name: "Make the change" }));
    await waitFor(() => expect(done).toHaveBeenCalled());
    expect(writes).toEqual([{ path: "reports-to", body: { reportsToId: "lead", from: "2026-12-01" } }]);
  });

  it("proposes moving a head's unit into the other's, and says why it cannot", async () => {
    const { writes, done } = dropped(card("head", "Head of Bench", "bench", "Bench", true), card("hoq", "Head of Quality", "quality", "Quality", true));
    expect(screen.getByLabelText("Bench moves into Quality")).toBeChecked();
    await userEvent.click(screen.getByRole("button", { name: "Make the change" }));
    await waitFor(() => expect(writes[0]?.path).toBe("move"));
    expect(writes[0]?.body).toMatchObject({ parentId: "quality" });
    expect(done).toHaveBeenCalled();
    cleanup();
    const refused = dropped(card("lh", "Lab head", "lab", "Lab", true), card("x", "Inside", "inner", "Inner"));
    await userEvent.click(screen.getByRole("button", { name: "Make the change" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("A unit cannot sit inside itself.");
    await userEvent.click(screen.getByLabelText("Lab head reports to Inside"));
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(refused.done).toHaveBeenCalled();
  });
});
