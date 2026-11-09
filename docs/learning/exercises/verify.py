#!/usr/bin/env python3
"""Verify exact downloadable lesson files in isolated copies of the module."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

sys.dont_write_bytecode = True
from prepare import BASE, prepare


def run(command, cwd, env):
    result = subprocess.run(command, cwd=cwd, env=env, text=True,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=180)
    return result.returncode, result.stdout


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--go", default="go")
    parser.add_argument("--days", type=int, nargs="+", default=list(range(9, 16)))
    parser.add_argument("--report", help="可选：写入 JSON 验证记录")
    args = parser.parse_args()
    if not args.days or any(day not in range(9, 16) for day in args.days):
        parser.error("days must be between 9 and 15")
    go = str(Path(args.go).resolve()) if "/" in args.go else args.go
    work = Path(tempfile.mkdtemp(prefix="learning-verify-"))
    env = dict(os.environ)
    env.setdefault("GOCACHE", str(work / "cache"))
    env.setdefault("GOMODCACHE", str(work / "modcache"))
    code, version = run([go, "version"], BASE, env)
    if code:
        raise RuntimeError(version)
    report = {"go": version.strip(), "days": {}}
    for day in args.days:
        red, lesson = prepare(day, work / f"day{day:02}-red")
        green, _ = prepare(day, work / f"day{day:02}-green", reference=True)
        packages = lesson["package"].split()
        compile_code, compile_log = run([go, "test", *packages, "-run", "^$"], red, env)
        red_code, red_log = run([go, "test", *packages, "-run", lesson["pattern"], "-count=1", "-v"], red, env)
        green_code, green_log = run([go, "test", "-race", *packages, "-run", lesson["pattern"], "-count=1", "-v"], green, env)
        result = {"compile": compile_code, "red": red_code, "green_race": green_code,
                  "expected_todo": "panic: TODO:" in red_log, "target_tests_ran": "=== RUN" in green_log,
                  "compile_log": compile_log, "red_log": red_log, "green_log": green_log}
        report["days"][str(day)] = result
        print(f"Day {day:02}: compile={compile_code} red={red_code} green/race={green_code}", flush=True)
        if compile_code or red_code == 0 or not result["expected_todo"] or green_code or not result["target_tests_ran"]:
            print(compile_log, red_log, green_log)
            raise RuntimeError(f"Day {day} validation failed")
    cumulative_code, cumulative_log = run([go, "test", "./...", "-timeout", "60s"], green, env)
    report["cumulative"] = {"command": "go test ./... -timeout 60s",
                            "mode": "compatible reference in isolated copy, including matched cmd baseline",
                            "exit_code": cumulative_code, "log": cumulative_log}
    print(f"Cumulative reference copy: {cumulative_code}", flush=True)
    if cumulative_code:
        raise RuntimeError(cumulative_log)
    if args.report:
        Path(args.report).write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print("Verified exact skeletons and unchanged behavioral tests; learner code was not touched.")


if __name__ == "__main__":
    main()
