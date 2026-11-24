#!/usr/bin/env bash
# Usage: paper/pilot/run.sh <suite.yaml> <outdir>
# Runs one pilot suite with the pinned examples/bin/agentchaos and records
# provenance.txt in <outdir>.
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: $0 <suite.yaml> <outdir>" >&2
  exit 2
fi
suite=$1
out=$2

root=$(cd "$(dirname "$0")/../.." && pwd)
cd "$root"

if [ -e "$out" ]; then
  echo "refusing: $out already exists" >&2
  exit 1
fi
mkdir -p "$out"

{
  echo "git_commit: $(git rev-parse HEAD)"
  echo "git_describe: $(git describe --tags --dirty)"
  echo "agentchaos: $(examples/bin/agentchaos version)"
  echo "claude: $(claude --version)"
  echo "model: $(grep -o -- '--model","[^"]*' "$suite" | head -1 | sed 's/.*,"//')"
  echo "suite: $suite"
  echo "start_utc: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
} > "$out/provenance.txt"

status=0
examples/bin/agentchaos run "$suite" --out "$out" || status=$?

echo "end_utc: $(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$out/provenance.txt"
echo "exit_status: $status" >> "$out/provenance.txt"
exit "$status"
