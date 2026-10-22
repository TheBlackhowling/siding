import { useEffect, useState } from "react";
import type { BoardStatus } from "./types";

const POLL_MS = 15_000;

function isBoardStatus(value: unknown): value is BoardStatus {
  if (value === null || typeof value !== "object") {
    return false;
  }
  const rec = value as Record<string, unknown>;
  return typeof rec.listen === "number" && Array.isArray(rec.stacks);
}

export function useBoardStatus() {
  const [status, setStatus] = useState<BoardStatus | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const res = await fetch("/status.json", { cache: "no-store" });
        if (!res.ok) {
          throw new Error(`status.json ${res.status}`);
        }
        const data: unknown = await res.json();
        if (!isBoardStatus(data)) {
          throw new Error("status.json is not a board payload");
        }
        if (!cancelled) {
          setStatus(data);
          setError("");
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "could not load status.json");
        }
      }
    }

    void load();
    const id = window.setInterval(() => {
      void load();
    }, POLL_MS);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, []);

  return { status, error };
}
