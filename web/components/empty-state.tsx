const README = "https://github.com/hamzafer/unpause#use-from-chatgpt";

/** Shown under the headline when the access log does not exist yet. */
export function EmptyState() {
  return (
    <div className="flex max-w-2xl flex-col gap-3 rounded-lg border bg-card p-5 text-sm">
      <p className="text-muted-foreground">
        Every call an MCP client makes to <code className="font-mono text-foreground">unpause mcp</code>{" "}
        shows up here. To connect one:
      </p>
      <ul className="flex flex-col gap-2">
        <li>
          <span className="font-medium">Claude Desktop</span>
          <span className="text-muted-foreground">: add </span>
          <code className="font-mono text-xs">unpause mcp</code>
          <span className="text-muted-foreground"> as a local MCP server in its settings.</span>
        </li>
        <li>
          <span className="font-medium">Codex</span>
          <span className="text-muted-foreground">: </span>
          <code className="font-mono text-xs">codex mcp add unpause -- unpause mcp</code>
        </li>
        <li>
          <span className="font-medium">ChatGPT</span>
          <span className="text-muted-foreground">: run it through OpenAI&apos;s Secure MCP Tunnel.</span>
        </li>
      </ul>
      <p>
        <a
          href={README}
          target="_blank"
          rel="noreferrer"
          className="text-ui-accent underline-offset-4 hover:underline"
        >
          Setup steps in the README
        </a>
      </p>
    </div>
  );
}
