#!/usr/bin/env python3
"""Development task runner for the grade backend.

Requirements (documented, not enforced): Go 1.26.5 and Python 3.8+ on PATH.
Docker only runs the local test database (compose.yaml); the app itself
always runs with `go run` for now.

Usage:
    python3 scripts/dev.py run     start the API server
    python3 scripts/dev.py test    run the test suite with the race detector
    python3 scripts/dev.py testdb  run the Postgres integration suite
    python3 scripts/dev.py lint    check formatting and vet the code
    python3 scripts/dev.py tidy    tidy the Go module
"""

import argparse
import os
import pathlib
import shutil
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent

# DSN of the compose.yaml database. Used as fallback by testdb.
COMPOSE_DSN = "postgres://grade:grade@localhost:5433/gradedb?sslmode=disable"


def require(program):
    if shutil.which(program) is None:
        sys.exit(
            f"error: '{program}' not found on PATH (see README for requirements)"
        )


def run(args, **kwargs):
    return subprocess.run(args, cwd=ROOT, **kwargs)


def cmd_run(_args):
    require("go")
    return run(["go", "run", "./src/cmd/server"]).returncode


def cmd_test(_args):
    require("go")
    return run(["go", "test", "./...", "-race", "-count=1"]).returncode


def cmd_testdb(_args):
    require("go")
    env = dict(os.environ)
    env.setdefault("TEST_DATABASE_URL", COMPOSE_DSN)
    proc = subprocess.run(
        ["go", "test", "./src/tests/postgres/", "-race", "-count=1", "-v"],
        cwd=ROOT,
        env=env,
    )
    return proc.returncode


def cmd_lint(_args):
    require("go")
    fmt = run(["gofmt", "-l", "."], capture_output=True)
    if fmt.stdout:
        print("gofmt: unformatted files:")
        print(fmt.stdout.decode())
    vet = run(["go", "vet", "./..."])
    if fmt.stdout or vet.returncode != 0:
        return 1
    print("lint: ok")
    return 0


def cmd_tidy(_args):
    require("go")
    return run(["go", "mod", "tidy"]).returncode


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    sub.add_parser("run", help="start the API server")
    sub.add_parser("test", help="run the test suite")
    sub.add_parser("testdb", help="run the Postgres integration suite")
    sub.add_parser("lint", help="format and vet")
    sub.add_parser("tidy", help="tidy the Go module")

    commands = {
        "run": cmd_run,
        "test": cmd_test,
        "testdb": cmd_testdb,
        "lint": cmd_lint,
        "tidy": cmd_tidy,
    }
    sys.exit(commands[parser.parse_args().command](parser))


if __name__ == "__main__":
    main()
