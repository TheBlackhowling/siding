export type CommandFlag = {
  name: string;
  def: string;
  detail: string;
};

export type CommandDoc = {
  slug: string;
  title: string;
  summary: string;
  usesPath?: boolean;
  usesMode?: boolean;
  flags: CommandFlag[];
  defaults: string[];
  selected: string[];
};

const pathFlag: CommandFlag = {
  name: "--path",
  def: "cwd",
  detail: "Optional. Walks up from this directory to .siding/. Omit it when you are already in the repo.",
};

const modeFlag = (def: string, detail: string): CommandFlag => ({
  name: "--mode",
  def,
  detail,
});

export const commands: CommandDoc[] = [
  {
    slug: "init",
    title: "siding init",
    summary: "Write a committed org recipe, or sync an existing one onto this machine. Does not start containers.",
    usesPath: true,
    flags: [
      pathFlag,
      { name: "--project", def: "folder prefix", detail: "Product id shared by every clone" },
      { name: "--folder-prefix", def: "guessed", detail: "Stripped from folder names (acme-betty → betty)" },
      { name: "--http-prefix", def: "inferred or none", detail: "Product prefix without app." },
      { name: "--entry KEY=LABEL", def: "web=app / api=api", detail: "Clickable board hosts" },
      { name: "--prefer-proxy", def: "true", detail: "Public URLs via Caddy on :80" },
      { name: "--rewrite-compose", def: "false", detail: "Adapt copies under .siding/compose/" },
      { name: "--force", def: "false", detail: "Overwrite an existing recipe" },
      { name: "--dry-run", def: "false", detail: "Show the plan; write nothing" },
    ],
    defaults: ["siding init --dry-run", "siding init"],
    selected: ["siding init --path {path} --dry-run", "siding init --path {path}"],
  },
  {
    slug: "sync",
    title: "siding sync",
    summary: "Map the committed recipe onto this checkout: allocate ports, write .siding/env, reload the proxy if it is up.",
    usesPath: true,
    usesMode: true,
    flags: [
      pathFlag,
      { name: "--stack", def: "from folder", detail: "Override stack name" },
      modeFlag("all modes", "Omit to allocate every mode; pass a name to sync one"),
      { name: "--dry-run", def: "false", detail: "Show mapping; do not write" },
      { name: "--json", def: "false", detail: "JSON instead of text" },
    ],
    defaults: ["siding sync", "siding sync --dry-run"],
    selected: ["siding sync --path {path}", "siding sync --path {path} --mode {mode}"],
  },
  {
    slug: "doctor",
    title: "siding doctor",
    summary: "Check install, recipe, compose wiring, Docker Compose, and whether this checkout is in state.json.",
    usesPath: true,
    flags: [pathFlag, { name: "--json", def: "false", detail: "JSON report" }],
    defaults: ["siding doctor"],
    selected: ["siding doctor --path {path}"],
  },
  {
    slug: "plan",
    title: "siding plan",
    summary: "Resolve stack, host, ports, and slot env without starting containers or writing state.",
    usesPath: true,
    usesMode: true,
    flags: [
      pathFlag,
      { name: "--stack", def: "from folder", detail: "Override stack name" },
      modeFlag("test", "Recipe mode"),
      { name: "--json", def: "false", detail: "JSON including slot_env" },
    ],
    defaults: ["siding plan", "siding plan --json"],
    selected: ["siding plan --path {path} --mode {mode}"],
  },
  {
    slug: "add",
    title: "siding add",
    summary: "Pre-allocate globally unique host ports. Optional — the first up also allocates.",
    usesPath: true,
    usesMode: true,
    flags: [pathFlag, modeFlag("test", "Recipe mode"), { name: "--dry-run", def: "false", detail: "Print allocation; do not write" }],
    defaults: ["siding add"],
    selected: ["siding add --path {path} --mode {mode}"],
  },
  {
    slug: "up",
    title: "siding up",
    summary: "Write slot env, claim the lock, start compose on the host, reload the proxy if it is running.",
    usesPath: true,
    usesMode: true,
    flags: [
      pathFlag,
      modeFlag("test", "Which recipe mode / compose project to start"),
      { name: "--build", def: "false", detail: "Pass --build to compose up" },
      { name: "--force-recreate", def: "false", detail: "Recreate containers" },
      { name: "--", def: "", detail: "Extra docker compose up args" },
    ],
    defaults: ["siding up", "siding up --build --force-recreate"],
    selected: ["siding up --path {path} --mode {mode}"],
  },
  {
    slug: "down",
    title: "siding down",
    summary: "Stop compose, release the lock, keep port allocations for the next up.",
    usesPath: true,
    usesMode: true,
    flags: [pathFlag, modeFlag("test", "Which recipe mode / compose project to stop"), { name: "-v / --volumes", def: "false", detail: "Also remove named volumes" }],
    defaults: ["siding down", "siding down -v"],
    selected: ["siding down --path {path} --mode {mode}"],
  },
  {
    slug: "recycle",
    title: "siding recycle",
    summary: "Stop then start the stack for a mode. Keeps volumes unless you pass -v.",
    usesPath: true,
    usesMode: true,
    flags: [
      pathFlag,
      modeFlag("test", "Which recipe mode / compose project to recycle"),
      { name: "--build", def: "false", detail: "Pass --build to compose up" },
      { name: "--force-recreate", def: "false", detail: "Recreate containers even if config is unchanged" },
      { name: "-v / --volumes", def: "false", detail: "Also remove named volumes on down" },
    ],
    defaults: ["siding recycle", "siding recycle --build --force-recreate"],
    selected: ["siding recycle --path {path} --mode {mode}"],
  },
  {
    slug: "reset",
    title: "siding reset",
    summary: "Wipe volumes and bring the stack back up rebuilt.",
    usesPath: true,
    usesMode: true,
    flags: [pathFlag, modeFlag("test", "Which recipe mode / compose project to reset")],
    defaults: ["siding reset"],
    selected: ["siding reset --path {path} --mode {mode}"],
  },
  {
    slug: "worktree-add",
    title: "siding worktree add",
    summary: "Create a sibling git worktree and siding sync it (does not up).",
    usesPath: true,
    usesMode: false,
    flags: [pathFlag],
    defaults: ["siding worktree add NAME BRANCH", "siding worktree add NAME BRANCH main"],
    selected: ["siding worktree add {stack} feat/{stack}"],
  },
  {
    slug: "worktree-remove",
    title: "siding worktree remove",
    summary: "down -v, remove from state, then git worktree remove. By NAME from the main checkout, or from the linked folder.",
    usesPath: true,
    usesMode: false,
    flags: [pathFlag, { name: "--force", def: "false", detail: "Continue after compose errors" }],
    defaults: ["siding worktree remove {stack}", "siding worktree remove"],
    selected: ["siding worktree remove {stack}"],
  },
  {
    slug: "worktree-list",
    title: "siding worktree list",
    summary: "Git worktrees with siding stack and up/allocated status.",
    usesPath: true,
    usesMode: false,
    flags: [pathFlag],
    defaults: ["siding worktree list"],
    selected: ["siding worktree list --path {path}"],
  },
  {
    slug: "worktree-sync",
    title: "siding worktree sync",
    summary: "Run siding sync on git worktrees missing from state.json (e.g. after plain git worktree add).",
    usesPath: true,
    usesMode: false,
    flags: [pathFlag, { name: "--dry-run", def: "false", detail: "Show which checkouts would be synced" }],
    defaults: ["siding worktree sync"],
    selected: ["siding worktree sync --path {path}"],
  },
  {
    slug: "tail",
    title: "siding tail",
    summary:
      "Follow compose logs for this checkout’s project. The project comes from cwd (or --path) plus --mode — not an image name. Extra arguments are compose service names (web, api).",
    usesPath: true,
    usesMode: true,
    flags: [
      pathFlag,
      modeFlag("test", "Which recipe mode / compose project to follow"),
      { name: "service…", def: "all services", detail: "Compose service names in that project (web, api). Not image names." },
      { name: "--tail", def: "100", detail: "Lines per container from the end" },
      { name: "--since", def: "", detail: "Timestamp or relative time (10m)" },
    ],
    defaults: ["siding tail", "siding tail web", "siding tail web api --since 10m"],
    selected: ["siding tail --path {path} --mode {mode}", "siding tail --path {path} --mode {mode} web"],
  },
  {
    slug: "ps",
    title: "siding ps",
    summary: "docker compose ps for this checkout’s project (cwd or --path, plus --mode).",
    usesPath: true,
    usesMode: true,
    flags: [pathFlag, modeFlag("test", "Which recipe mode / compose project")],
    defaults: ["siding ps"],
    selected: ["siding ps --path {path} --mode {mode}"],
  },
  {
    slug: "remove",
    title: "siding remove",
    summary: "Down each mode, then drop this checkout’s allocations and locks. Does not delete repo files.",
    usesPath: true,
    usesMode: true,
    flags: [
      pathFlag,
      modeFlag("all modes", "Forget one mode only"),
      { name: "--dry-run", def: "false", detail: "Print what would be forgotten" },
      { name: "--force", def: "false", detail: "Drop state even if compose down fails" },
      { name: "--volumes", def: "false", detail: "Wipe named volumes, then forget" },
    ],
    defaults: ["siding remove", "siding remove --dry-run"],
    selected: ["siding remove --path {path}", "siding remove --path {path} --mode {mode}"],
  },
  {
    slug: "proxy",
    title: "siding proxy",
    summary: "Machine-wide Caddy. Routes come from all checkouts in state.json, not from --path.",
    flags: [],
    defaults: ["siding proxy up", "siding proxy reload", "siding proxy down", "siding proxy ps"],
    selected: [],
  },
  {
    slug: "proxy-up",
    title: "siding proxy up",
    summary: "Start or recreate the proxy. Pulls ghcr.io/theblackhowling/siding-proxy:latest so the board UI can move without a new CLI.",
    flags: [{ name: "--listen", def: "80", detail: "Host HTTP port" }],
    defaults: ["siding proxy up", "siding proxy up --listen 1365"],
    selected: [],
  },
  {
    slug: "install",
    title: "siding install",
    summary: "Copy this executable onto your user PATH so PowerShell can run siding.",
    flags: [],
    defaults: ["siding install"],
    selected: [],
  },
  {
    slug: "version",
    title: "siding version",
    summary: "Print version, commit, build date, and Go version.",
    flags: [],
    defaults: ["siding version"],
    selected: [],
  },
];

export function commandBySlug(slug: string): CommandDoc | undefined {
  return commands.find((c) => c.slug === slug);
}

export function quotePath(path: string): string {
  const trimmed = path.trim();
  if (!trimmed) {
    return "";
  }
  if (/[\s&]/.test(trimmed)) {
    return `"${trimmed}"`;
  }
  return trimmed;
}

export function fillExample(template: string, path: string, mode: string): string {
  return template.replaceAll("{path}", quotePath(path)).replaceAll("{mode}", mode.trim() || "test");
}

export function stackCommand(bin: string, path: string, mode: string): string {
  const parts = ["siding", bin, "--path", quotePath(path)];
  if (mode) {
    parts.push("--mode", mode);
  }
  return parts.join(" ");
}

export function commandHref(slug: string, path: string, mode: string): string {
  const q = new URLSearchParams();
  if (path) {
    q.set("path", path);
  }
  if (mode) {
    q.set("mode", mode);
  }
  const suffix = q.toString();
  return suffix ? `/commands/${slug}?${suffix}` : `/commands/${slug}`;
}

export function selectionQuery(path: string, mode: string): string {
  const q = new URLSearchParams();
  if (path) {
    q.set("path", path);
  }
  if (mode) {
    q.set("mode", mode);
  }
  const suffix = q.toString();
  return suffix ? `?${suffix}` : "";
}
