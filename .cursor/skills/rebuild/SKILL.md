---
name: rebuild
description: Rebuild the host siding binary via Docker and install it on PATH. Use when the user asks to rebuild, build the exe, refresh bin/siding.exe, install, or run /rebuild.
---

# Rebuild siding

There is no local Go on the Windows host. Cross-compile through Docker, then copy the exe onto the user PATH.

From the repo root:

```powershell
.\scripts\install.ps1 -Build
```

That writes `bin\siding.exe` (windows/amd64) and copies it to `%LOCALAPPDATA%\siding\bin\siding.exe`. Needs Docker Desktop. Give the shell full permissions so compose can run.

After it finishes, report both paths. A local build prints `siding dev` / `commit: none` — that is expected (no release ldflags).

`install.ps1` prepends the bin dir for **this** shell session. Cursor's integrated terminal still has the IDE's PATH until Cursor restarts; if `siding` is not found after install, print:

```powershell
$env:Path = "$env:LOCALAPPDATA\siding\bin;" + $env:Path
```

If they ask for every platform: `.\docker\build.ps1 -All` then `.\scripts\install.ps1`.
