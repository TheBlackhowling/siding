import { useMemo, useState } from "react";
import type { BoardStack } from "./types";
import { commandHref, stackCommand } from "./commands";
import { copyText } from "./copy";
import { groupByProject, type ProjectGroup, type StackGroup } from "./boardGroups";
import { useBoardStatus } from "./useBoardStatus";

function formatLastUp(raw: string): string {
  if (!raw) {
    return "";
  }
  const t = new Date(raw);
  if (Number.isNaN(t.getTime())) {
    return "";
  }
  return t.toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
}

function linkHost(url: string): string {
  return url.replace(/^https?:\/\//, "");
}

function ModeRow({ row }: { row: BoardStack }) {
  const [copied, setCopied] = useState(false);
  const upCmd = stackCommand("up", row.checkout, row.mode);
  const git = [row.branch, row.sha ? `@ ${row.sha}` : ""].filter(Boolean).join(" ");
  const upLine = formatLastUp(row.last_up_at);

  return (
    <div className={`mode${row.running ? " mode-up" : ""}`}>
      <div className="mode-top">
        <div className="mode-id">
          <span className="mode-name">{row.mode || "default"}</span>
          <span className={row.running ? "badge up" : "badge down"}>{row.running ? "up" : "allocated"}</span>
        </div>
        <div className="mode-actions">
          {(["up", "down", "tail", "ps", "doctor"] as const).map((slug) => (
            <a key={slug} href={commandHref(slug, row.checkout, row.mode)}>
              {slug}
            </a>
          ))}
          <button
            type="button"
            className="copy copy-quiet"
            onClick={() => {
              void copyText(upCmd).then((ok) => {
                if (ok) {
                  setCopied(true);
                  window.setTimeout(() => setCopied(false), 1400);
                }
              });
            }}
          >
            {copied ? "copied" : "copy up"}
          </button>
        </div>
      </div>
      <div className="mode-meta">
        {row.host ? <span>{row.host}</span> : null}
        {row.compose_project ? <span>compose {row.compose_project}</span> : null}
        {git ? <span>git {git}{upLine ? ` · ${upLine}` : ""}</span> : null}
      </div>
      <div className="pills">
        {(row.links ?? []).length === 0 ? <span className="empty">no HTTP links</span> : null}
        {(row.links ?? []).map((link) => (
          <a key={`${link.label}:${link.url}`} className="pill" href={link.url}>
            <span>{link.label}</span>
            {linkHost(link.url)}
          </a>
        ))}
      </div>
    </div>
  );
}

function StackBlock({ group, showName }: { group: StackGroup; showName: boolean }) {
  return (
    <div className="stack">
      {showName ? (
        <div className="stack-head">
          <div className="stack-name">{group.stack}</div>
          <div className="stack-path">{group.checkout}</div>
        </div>
      ) : (
        <div className="stack-path lone">{group.checkout}</div>
      )}
      {group.rows.map((row) => (
        <ModeRow key={`${row.checkout}:${row.mode}`} row={row} />
      ))}
    </div>
  );
}

function ProjectCard({ group }: { group: ProjectGroup }) {
  const [open, setOpen] = useState(false);
  const multi = group.stacks.length > 1 || group.stacks.some((s) => s.stack && s.stack !== group.project);
  return (
    <article className={`project${open ? " open" : ""}${group.up > 0 ? " has-up" : ""}`}>
      <button type="button" className="project-toggle" aria-expanded={open} onClick={() => setOpen((v) => !v)}>
        <span className="chevron" aria-hidden>
          {open ? "▾" : "▸"}
        </span>
        <span className="project-name">{group.project}</span>
        <span className="project-counts">
          {group.up > 0 ? <span className="count-up">{group.up} up</span> : null}
          {group.allocated > 0 ? <span className="count-down">{group.allocated} allocated</span> : null}
        </span>
      </button>
      {open ? (
        <div className="project-body">
          {group.stacks.map((stack) => (
            <StackBlock key={`${stack.stack}:${stack.checkout}`} group={stack} showName={multi} />
          ))}
        </div>
      ) : null}
    </article>
  );
}

export function BoardPage() {
  const { status, error } = useBoardStatus();
  const groups = useMemo(() => groupByProject(status?.stacks ?? []), [status]);

  return (
    <>
      <header className="page-head">
        <h1>Board</h1>
        <p className="sub">Grouped by product. A card is up when that slot lock is held.</p>
      </header>
      {error && !status ? <p className="error">{error}</p> : null}
      {status && status.stacks.length === 0 ? (
        <p className="empty">
          No checkouts yet. Run <code>siding init</code> then <code>siding up</code>.{" "}
          <a href="/commands/init">Commands</a>
        </p>
      ) : null}
      {groups.map((group) => (
        <ProjectCard key={group.project} group={group} />
      ))}
    </>
  );
}
