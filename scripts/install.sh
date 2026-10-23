#!/usr/bin/env bash
# Install siding onto PATH for this user and this shell session.
#
# Release layout (extracted tarball/zip):
#   ./install.sh
#
# Dev repo:
#   ./scripts/install.sh
#
set -euo pipefail

case "$(uname -s 2>/dev/null || echo unknown)" in
MINGW* | MSYS* | CYGWIN*)
	echo "On Windows use: .\\scripts\\install.ps1" >&2
	exit 1
	;;
esac

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bin=""
if [[ -f "$script_dir/siding" ]]; then
	bin="$script_dir/siding"
elif [[ -f "$script_dir/../bin/siding" ]]; then
	bin="$script_dir/../bin/siding"
else
	echo "siding binary not found next to install.sh or in bin/siding" >&2
	exit 1
fi

chmod +x "$bin"
"$bin" install

bin_dir="${HOME}/.local/bin"
export PATH="${bin_dir}:${PATH}"

if command -v siding >/dev/null 2>&1; then
	siding version
else
	"${bin_dir}/siding" version
fi

echo ""
echo "This session: siding is on PATH."
echo "New terminals pick up PATH from your shell config (~/.zprofile / ~/.zshrc on macOS)."
