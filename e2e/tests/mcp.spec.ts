import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StreamableHTTPClientTransport } from "@modelcontextprotocol/sdk/client/streamableHttp.js";
import { expect, test } from "@playwright/test";
import { connectApp } from "../fixtures/oauth";
import { signIn } from "../fixtures/session";

type Structured = Record<string, unknown>;

// The official MCP client, as an assistant uses it, against the stack's MCP endpoint with a token from the consent page.
test("an assistant asks the demo organisation who approves the supplier invoice over MCP", async ({ page, baseURL }) => {
  await signIn(page);
  const token = await connectApp(page, `MCP client ${Date.now()}`);
  const transport = new StreamableHTTPClientTransport(new URL("/api/v1/mcp", baseURL), {
    requestInit: { headers: { Authorization: `Bearer ${token}` } },
  });
  const client = new Client({ name: "bearing-e2e", version: "1.0.0" });
  await client.connect(transport);
  try {
    expect(client.getServerVersion()?.name).toBe("bearing");
    const { tools } = await client.listTools();
    expect(tools.map((t) => t.name)).toEqual(expect.arrayContaining(["whoami", "search", "who_is_responsible", "get_process"]));

    const me = (await client.callTool({ name: "whoami", arguments: {} })).structuredContent as Structured;
    expect((me.account as Structured).email).toBe("admin@bearing.test");

    const answer = await client.callTool({ name: "who_is_responsible", arguments: { query: "approve the invoice" } });
    expect(answer.isError).toBeFalsy();
    const [step] = (answer.structuredContent as { steps: { name: string; responsibilities: { kind: string; people: { name: string }[] }[] }[] }).steps;
    expect(step?.name).toBe("Approve the invoice");
    const accountable = step?.responsibilities.find((r) => r.kind === "accountable");
    expect(accountable?.people.length).toBeGreaterThan(0);

    const refused = await client.callTool({ name: "get_person", arguments: { personId: "nobody" } });
    expect(refused.isError).toBe(true);
  } finally {
    await client.close();
  }
});
