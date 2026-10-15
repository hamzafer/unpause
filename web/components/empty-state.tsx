const README = "https://github.com/hamzafer/unpause";
const DESKTOP_CODEX = `${README}#use-from-claude-desktop-or-codex`;
const CHATGPT = `${README}#use-from-chatgpt`;

const linkClass = "text-ui-accent underline-offset-4 hover:underline";

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
          <span className="text-muted-foreground"> to </span>
          <code className="font-mono text-xs">claude_desktop_config.json</code>
          <span className="text-muted-foreground"> and restart it. It works in Cowork too.</span>
        </li>
        <li>
          <span className="font-medium">Codex</span>
          <span className="text-muted-foreground">: </span>
          <code className="font-mono text-xs">codex mcp add unpause -- unpause mcp</code>
        </li>
        <li>
          <span className="font-medium">ChatGPT</span>
          <span className="text-muted-foreground">: run it through OpenAI&apos;s Secure MCP Tunnel (</span>
          <a href={CHATGPT} target="_blank" rel="noreferrer" className={linkClass}>
            steps
          </a>
          <span className="text-muted-foreground">).</span>
        </li>
      </ul>
      <p>
        <a href={DESKTOP_CODEX} target="_blank" rel="noreferrer" className={linkClass}>
          Claude Desktop and Codex setup in the README
        </a>
      </p>
    </div>
  );
}
