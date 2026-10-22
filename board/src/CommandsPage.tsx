import { useState } from "react";
import { commandBySlug, commandHref, commands, fillExample, selectionQuery } from "./commands";
import { copyText } from "./copy";
import { StackSelector } from "./StackSelector";
import { useBoardStatus } from "./useBoardStatus";

function useQuery(): URLSearchParams {
  return new URLSearchParams(window.location.search);
}

function CopyBlock({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <div className="cmd-row">
      <code>{text}</code>
      <button
        type="button"
        className="copy"
        onClick={() => {
          void copyText(text).then((ok) => {
            if (ok) {
              setCopied(true);
              window.setTimeout(() => setCopied(false), 1500);
            }
          });
        }}
      >
        {copied ? "copied" : "copy"}
      </button>
    </div>
  );
}

function ExampleGroup({ title, lines }: { title: string; lines: string[] }) {
  if (lines.length === 0) {
    return null;
  }
  return (
    <section className="examples">
      <h2>{title}</h2>
      {lines.map((line) => (
        <CopyBlock key={line} text={line} />
      ))}
    </section>
  );
}

function CommandsHeader({
  path,
  mode,
  onSelect,
}: {
  path: string;
  mode: string;
  onSelect: (path: string, mode: string) => void;
}) {
  const { status } = useBoardStatus();
  return (
    <StackSelector
      stacks={status?.stacks ?? []}
      path={path}
      mode={mode}
      onChange={(nextPath, nextMode) => {
        onSelect(nextPath, nextMode);
        const here = window.location.pathname.replace(/\/+$/, "") || "/commands";
        window.history.replaceState(null, "", here + selectionQuery(nextPath, nextMode));
      }}
    />
  );
}

export function CommandsIndex({ path, mode, onSelect }: { path: string; mode: string; onSelect: (p: string, m: string) => void }) {
  return (
    <>
      <header className="page-head">
        <h1>Commands</h1>
        <p className="sub">
          From a checkout, <code>--path</code> is optional. Pick a siding to fill the specific lines.
        </p>
      </header>
      <CommandsHeader path={path} mode={mode} onSelect={onSelect} />
      {commands.map((cmd) => (
        <article className="card" key={cmd.slug}>
          <header>
            <a className="name" href={commandHref(cmd.slug, path, mode)}>
              {cmd.title}
            </a>
          </header>
          <div className="meta">{cmd.summary}</div>
        </article>
      ))}
    </>
  );
}

export function CommandDetail({
  slug,
  path,
  mode,
  onSelect,
}: {
  slug: string;
  path: string;
  mode: string;
  onSelect: (p: string, m: string) => void;
}) {
  const cmd = commandBySlug(slug);

  if (!cmd) {
    return (
      <>
        <h1>Unknown command</h1>
        <p className="empty">
          No page for <code>{slug}</code>. <a href={`/commands${selectionQuery(path, mode)}`}>All commands</a>
        </p>
      </>
    );
  }

  const selected =
    path && cmd.selected.length > 0 ? cmd.selected.map((ex) => fillExample(ex, path, mode)) : [];

  return (
    <>
      <p className="crumb">
        <a href={`/commands${selectionQuery(path, mode)}`}>Commands</a>
      </p>
      <h1>{cmd.title}</h1>
      <p className="sub">{cmd.summary}</p>
      <CommandsHeader path={path} mode={mode} onSelect={onSelect} />
      <ExampleGroup title="This directory" lines={cmd.defaults} />
      <ExampleGroup title={path ? `Selected · ${mode || "test"}` : "Selected siding"} lines={selected} />
      {!path && cmd.selected.length > 0 ? (
        <p className="empty">Pick a siding above to fill in --path and --mode.</p>
      ) : null}
      {cmd.flags.length > 0 ? (
        <table className="flags">
          <thead>
            <tr>
              <th>Flag</th>
              <th>Default</th>
              <th>What it does</th>
            </tr>
          </thead>
          <tbody>
            {cmd.flags.map((f) => (
              <tr key={f.name}>
                <td>
                  <code>{f.name}</code>
                </td>
                <td>{f.def || "—"}</td>
                <td>{f.detail}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
    </>
  );
}

export function CommandsPage() {
  const q = useQuery();
  const [path, setPath] = useState(q.get("path") ?? "");
  const [mode, setMode] = useState(q.get("mode") ?? "");
  const parts = window.location.pathname.replace(/\/+$/, "").split("/").filter(Boolean);
  const slug = parts[0] === "commands" ? parts[1] : "";
  const onSelect = (nextPath: string, nextMode: string) => {
    setPath(nextPath);
    setMode(nextMode);
  };
  if (slug) {
    return <CommandDetail slug={slug} path={path} mode={mode} onSelect={onSelect} />;
  }
  return <CommandsIndex path={path} mode={mode} onSelect={onSelect} />;
}
