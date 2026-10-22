import { BoardPage } from "./BoardPage";
import { CommandsPage } from "./CommandsPage";

function pathOf(): string {
  return window.location.pathname.replace(/\/+$/, "") || "/";
}

export function App() {
  const path = pathOf();
  const onCommands = path === "/commands" || path.startsWith("/commands/");

  return (
    <>
      <header className="top">
        <a className="brand" href="/">
          Siding
        </a>
        <nav className="nav">
          <a href="/" className={path === "/" ? "active" : ""}>
            Board
          </a>
          <a href="/commands" className={onCommands ? "active" : ""}>
            Commands
          </a>
        </nav>
      </header>
      <main>{onCommands ? <CommandsPage /> : <BoardPage />}</main>
    </>
  );
}
