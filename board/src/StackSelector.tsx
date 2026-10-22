import type { BoardStack } from "./types";

export function stackKey(path: string, mode: string): string {
  if (!path) {
    return "";
  }
  return `${path}\0${mode}`;
}

export function parseStackKey(key: string): { path: string; mode: string } {
  if (!key) {
    return { path: "", mode: "" };
  }
  const i = key.indexOf("\0");
  if (i < 0) {
    return { path: key, mode: "" };
  }
  return { path: key.slice(0, i), mode: key.slice(i + 1) };
}

export function stackLabel(row: BoardStack): string {
  const name = row.mode ? `${row.stack} · ${row.mode}` : row.stack;
  return `${name}  —  ${row.checkout}`;
}

type Props = {
  stacks: BoardStack[];
  path: string;
  mode: string;
  onChange: (path: string, mode: string) => void;
};

export function StackSelector({ stacks, path, mode, onChange }: Props) {
  return (
    <label className="selector">
      <span>Siding</span>
      <select
        value={stackKey(path, mode)}
        onChange={(e) => {
          const next = parseStackKey(e.target.value);
          onChange(next.path, next.mode);
        }}
      >
        <option value="">This directory (cwd)</option>
        {stacks.map((row) => (
          <option key={`${row.checkout}:${row.mode}`} value={stackKey(row.checkout, row.mode)}>
            {stackLabel(row)}
          </option>
        ))}
      </select>
    </label>
  );
}
