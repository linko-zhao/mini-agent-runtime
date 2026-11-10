#!/usr/bin/env python3
"""Create an isolated copy of the same cumulative module; never edit the source."""
import argparse
import json
from pathlib import Path
import shutil
import tempfile

BASE = Path(__file__).resolve().parent
REPO = BASE.parents[2]


def prepare(day, destination=None, reference=False):
    manifest = json.loads((BASE / "manifest.json").read_text(encoding="utf-8"))
    lesson = manifest[str(day)]
    if destination is None:
        out = Path(tempfile.mkdtemp(prefix=f"mini-agent-day{day:02}-"))
    else:
        out = Path(destination).resolve()
        if out == REPO or REPO in out.parents or out in REPO.parents:
            raise ValueError("练习副本必须位于原仓库外，不能覆盖原目录或父目录")
        if out.exists():
            raise ValueError("目标目录已存在，请使用一个新目录")
        out.mkdir(parents=True)
    shutil.copytree(REPO, out, dirs_exist_ok=True,
                    ignore=shutil.ignore_patterns(".git", "__pycache__", ".pytest_cache"))
    # Only this newly-created copy receives reference prerequisites.
    # One shared, versioned teaching baseline keeps earlier lessons stable as
    # main adds newer observation APIs. No module or protocol is redefined.
    for directory in ("internal", "cmd"):
        if (out / directory).exists():
            shutil.rmtree(out / directory)
        (out / directory).mkdir()
    if day <= 7:
        # Apply only earlier lessons, in course order. No future implementation
        # or learner source is used as a prerequisite for the first week.
        for previous in range(1, day):
            prior = manifest[str(previous)]
            for group in (prior["support"], prior["targets"], prior["tests"]):
                for item in group:
                    source = item.get("reference", item.get("file"))
                    target = out / item["path"]
                    target.parent.mkdir(parents=True, exist_ok=True)
                    target.write_bytes((BASE / source).read_bytes())
    else:
        for source in (BASE / "baseline").rglob("*.go.txt"):
            target = out / str(source.relative_to(BASE / "baseline"))[:-4]
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(source.read_bytes())
    for group in (lesson["support"], lesson["targets"], lesson["tests"]):
        for item in group:
            key = ("reference" if reference else "skeleton") if "skeleton" in item else "file"
            target = out / item["path"]
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes((BASE / item[key]).read_bytes())
    return out, lesson


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("day", type=int, choices=[*range(1, 8), *range(9, 16)])
    parser.add_argument("--output", help="可选：尚不存在、位于原仓库外的新目录")
    args = parser.parse_args()
    directory, lesson = prepare(args.day, args.output)
    print("练习副本（原实现保持不动）：")
    print(directory)
    print("在此副本根目录完成：")
    for item in lesson["targets"]:
        print("  " + item["path"])
    print("先编译：go test " + lesson["package"] + " -run '^$'")
    print("再看红灯：go test " + lesson["package"] + " -run '" + lesson["pattern"] + "' -count=1 -v")
