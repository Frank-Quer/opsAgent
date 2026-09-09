#!/usr/bin/env python3
"""Run the same checks locally and in CI; tests have a 60-second hard timeout."""

from pathlib import Path
import subprocess
import sys


def run(command, **kwargs):
    print("+ " + " ".join(command), flush=True)
    return subprocess.run(command, check=True, **kwargs)


def main():
    if sys.argv[1:] not in ([], ["test"]):
        sys.exit("Usage: python3 scripts/check.py [test]")
    if not sys.argv[1:]:
        files = ["main.go"] + sorted(str(p) for p in Path("internal").rglob("*.go"))
        result = run(["gofmt", "-l", *files], capture_output=True, text=True)
        if result.stdout:
            sys.exit("请先运行 gofmt：\n" + result.stdout)
        run(["go", "vet", "./..."])
        run(["bash", "-n", "service.sh"])
    run(["go", "test", "-race", "-timeout", "60s", "./..."], timeout=60)
    if not sys.argv[1:]:
        run(["go", "build", "-o", "bin/opsagent", "."])


if __name__ == "__main__":
    try:
        main()
    except subprocess.TimeoutExpired:
        sys.exit("测试超过 60 秒硬超时。")
    except subprocess.CalledProcessError as error:
        sys.exit(error.returncode)
