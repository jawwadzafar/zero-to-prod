"""Run dbt, pointed at the lake and the warehouse.

`snipdata build` and the orchestrator (chapter 17.5) both come through here,
so dbt always sees the same paths. dbt runs as a child process, the way an
orchestrator runs a task: each run starts clean and lets go of the warehouse
file when it exits. What happened is read back from the file dbt writes for
every run, target/run_results.json.
"""

from __future__ import annotations

import json
import os
import subprocess
import sys
from collections import Counter
from dataclasses import dataclass, field
from pathlib import Path

# dbt's exit codes: 0 everything passed, 1 a model or test failed, 2 dbt itself couldn't run.
MODEL_OR_TEST_FAILED = 1


@dataclass(frozen=True)
class Outcome:
    success: bool
    # Set when dbt itself couldn't run (a locked file, a bad profile): worth retrying.
    # Unset when dbt ran and a model or a test failed: rerunning won't change the data.
    error: str | None = None
    statuses: Counter[str] = field(default_factory=Counter)  # e.g. {"success": 7, "pass": 30}
    failed: list[str] = field(default_factory=list)  # names of failed models and tests
    warned: list[str] = field(default_factory=list)  # names of tests that warned


def run_dbt(
    lake: Path, warehouse: Path, dbt_dir: Path, args: list[str], quiet: bool = False
) -> Outcome:
    warehouse.parent.mkdir(parents=True, exist_ok=True)
    env = {
        **os.environ,
        "SNIPDATA_LAKE": str(lake.resolve()),
        "SNIPDATA_WAREHOUSE": str(warehouse.resolve()),
    }
    results = dbt_dir / "target" / "run_results.json"
    results.unlink(missing_ok=True)  # so a stale file from an earlier run can't be misread
    cmd = [sys.executable, "-c", "import sys; from dbt.cli.main import cli; sys.exit(cli())", *args]
    cmd += ["--project-dir", str(dbt_dir), "--profiles-dir", str(dbt_dir)]
    done = subprocess.run(cmd, env=env, capture_output=quiet, text=True)
    statuses: Counter[str] = Counter()
    failed: list[str] = []
    warned: list[str] = []
    if results.exists():
        for r in json.loads(results.read_text())["results"]:
            statuses[r["status"]] += 1
            name = r["unique_id"].split(".")[2]  # test.snip_analytics.<name>.<hash>
            if r["status"] in ("error", "fail"):
                failed.append(name)
            elif r["status"] == "warn":
                warned.append(name)
    if done.returncode not in (0, MODEL_OR_TEST_FAILED):
        lines = (done.stdout or done.stderr or "").strip().splitlines()
        return Outcome(False, error=lines[-1] if lines else f"dbt exited with {done.returncode}")
    return Outcome(done.returncode == 0, None, statuses, failed, warned)
