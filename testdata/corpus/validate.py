#!/usr/bin/env python3
"""Validate Docktree's Compose corpus without requiring project-specific app setup.

For each corpus project this runs:
  1. docker compose config
  2. docktree up --dry-run --json
  3. docker compose config with Docktree's generated override layered on top
  4. an optional bounded start probe using local images only (--pull never)

The start probe is intentionally conservative: it reports whether the stack can
start in the current Docker environment without pulling images or performing
project-specific provisioning.
"""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parent
REPO = ROOT.parent.parent


def run(cmd: list[str], *, cwd: Path = REPO, timeout: int = 120) -> tuple[bool, str]:
    try:
        proc = subprocess.run(
            cmd,
            cwd=cwd,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            timeout=timeout,
            check=False,
        )
    except subprocess.TimeoutExpired as exc:
        out = exc.stdout or ""
        return False, f"timed out after {timeout}s\n{out}".strip()
    return proc.returncode == 0, proc.stdout.strip()


def compose_cmd(project: Path, *args: str) -> list[str]:
    cmd = ["docker", "compose", "--project-directory", str(project)]
    env_file = project / ".env"
    if env_file.exists():
        cmd += ["--env-file", str(env_file)]
    cmd += ["-f", str(project / "compose.yml")]
    cmd += list(args)
    return cmd


def sanitize(value: str) -> str:
    line = " ".join(value.split())
    if len(line) <= 360:
        return line
    return f"{line[:160]} … {line[-180:]}"

def parse_json_object(output: str) -> dict[str, object]:
    for line in reversed(output.splitlines()):
        line = line.strip()
        if line.startswith("{") and line.endswith("}"):
            return json.loads(line)
    raise json.JSONDecodeError("no JSON object found", output, 0)



def validate_project(project: Path, *, start: bool, docktree_bin: str) -> dict[str, object]:
    result: dict[str, object] = {"project": project.name}

    ok, out = run(compose_cmd(project, "config"), timeout=120)
    result["compose_config"] = "ok" if ok else "fail"
    if not ok:
        result["compose_config_error"] = sanitize(out)
        result["docktree_dry_run"] = "skipped"
        result["override_config"] = "skipped"
        result["start_probe"] = "skipped"
        return result

    ok, out = run([docktree_bin, "--json", "up", "--dry-run", "-f", str(project / "compose.yml")], timeout=120)
    result["docktree_dry_run"] = "ok" if ok else "fail"
    if not ok:
        result["docktree_error"] = sanitize(out)
        result["override_config"] = "skipped"
        result["start_probe"] = "skipped"
        return result

    try:
        dry_run = parse_json_object(out)
        clear = dry_run.get("clear_preview", "")
        override = dry_run.get("override_preview", "")
        result["services"] = len(dry_run.get("services", []))
        result["published_ports"] = len(dry_run.get("ports", []))
        result["isolated_volumes"] = len(dry_run.get("isolated_volumes", []))
    except json.JSONDecodeError as exc:
        result["override_config"] = "fail"
        result["override_error"] = f"invalid docktree JSON: {exc}"
        result["start_probe"] = "skipped"
        return result

    temp_paths: list[Path] = []
    try:
        with tempfile.NamedTemporaryFile("w", suffix=".docktree.clear.yml", delete=False) as handle:
            handle.write(clear)
            clear_path = Path(handle.name)
            temp_paths.append(clear_path)
        with tempfile.NamedTemporaryFile("w", suffix=".docktree.override.yml", delete=False) as handle:
            handle.write(override)
            override_path = Path(handle.name)
            temp_paths.append(override_path)
        cmd = compose_cmd(project)
        cmd += ["-f", str(clear_path), "-f", str(override_path), "config"]
        ok, out = run(cmd, timeout=120)
    finally:
        if not start:
            for path in temp_paths:
                path.unlink(missing_ok=True)
    result["override_config"] = "ok" if ok else "fail"
    if not ok:
        result["override_error"] = sanitize(out)
        result["start_probe"] = "skipped"
        for path in temp_paths:
            path.unlink(missing_ok=True)
        return result

    if not start:
        result["start_probe"] = "not-run"
        return result

    project_name = f"docktree-corpus-{project.name}".replace("_", "-")[:63]
    up_cmd = ["docker", "compose", "--project-name", project_name, "--project-directory", str(project)]
    env_file = project / ".env"
    if env_file.exists():
        up_cmd += ["--env-file", str(env_file)]
    up_cmd += ["-f", str(project / "compose.yml"), "-f", str(clear_path), "-f", str(override_path), "up", "-d", "--wait", "--wait-timeout", "30", "--no-build", "--pull", "never"]
    ok, out = run(up_cmd, timeout=180)
    result["start_probe"] = "started" if ok else "not-started"
    if not ok:
        result["start_error"] = sanitize(out)

    down_cmd = ["docker", "compose", "--project-name", project_name, "--project-directory", str(project)]
    if env_file.exists():
        down_cmd += ["--env-file", str(env_file)]
    down_cmd += ["-f", str(project / "compose.yml"), "-f", str(clear_path), "-f", str(override_path), "down", "-v", "--remove-orphans"]
    down_ok, down_out = run(down_cmd, timeout=120)
    result["cleanup"] = "ok" if down_ok else "fail"
    if not down_ok:
        result["cleanup_error"] = sanitize(down_out)
    for path in temp_paths:
        path.unlink(missing_ok=True)
    return result


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--start", action="store_true", help="run bounded local-image start probes")
    parser.add_argument("--docktree-bin", default=str(REPO / "docktree"), help="docktree binary to test")
    parser.add_argument("--json-output", default="", help="optional path to write JSON results")
    args = parser.parse_args()

    if not shutil.which("docker"):
        print("docker not found", file=sys.stderr)
        return 1
    docktree_bin = args.docktree_bin
    if not Path(docktree_bin).exists():
        print(f"docktree binary not found: {docktree_bin}", file=sys.stderr)
        return 1

    results = []
    for project in sorted(p for p in ROOT.iterdir() if p.is_dir()):
        result = validate_project(project, start=args.start, docktree_bin=docktree_bin)
        results.append(result)
        print(json.dumps(result, sort_keys=True))

    if args.json_output:
        Path(args.json_output).write_text(json.dumps(results, indent=2, sort_keys=True) + "\n")

    failures = [
        r for r in results
        if r["compose_config"] != "ok" or r["docktree_dry_run"] != "ok" or r["override_config"] != "ok"
    ]
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
