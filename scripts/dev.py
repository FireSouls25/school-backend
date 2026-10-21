#!/usr/bin/env python3
"""Development task runner for the grade backend.

Requirements (documented, not enforced): Go 1.26.5 and Python 3.8+ on PATH.
Docker runs the local stack (Postgres + API); the app itself can also be
started directly with `go run`.

Usage:
    python3 scripts/dev.py up        # start the stack (db + api, demo data)
    python3 scripts/dev.py down      # stop the stack (add --volumes to wipe)
    python3 scripts/dev.py logs      # follow api logs
    python3 scripts/dev.py run       # run the API locally (PORT env, 8080)
    python3 scripts/dev.py seed      # create the demo data (idempotent)
    python3 scripts/dev.py test      # run the test suite with the race detector
    python3 scripts/dev.py testdb    # run the Postgres integration suite
    python3 scripts/dev.py lint      # check formatting and vet
    python3 scripts/dev.py tidy      # tidy the Go module
    python3 scripts/dev.py reset     # stop the stack and wipe its volume
"""

import argparse
import os
import pathlib
import shutil
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent

# DSN of the compose.yaml database. Used as fallback by testdb and run.
COMPOSE_DSN = "postgres://grade:grade@localhost:5433/gradedb?sslmode=disable"

# Origins the Vite dev server uses; needed so the SPA may call the API.
DEV_ORIGINS = "http://localhost:1420,http://127.0.0.1:1420"


def require(program):
    if shutil.which(program) is None:
        sys.exit(
            f"error: '{program}' not found on PATH (see README for requirements)"
        )


def require_docker():
    if shutil.which("docker") is None:
        sys.exit(
            "error: 'docker' not found on PATH. Install Docker, or use "
            "'dev.py run' with the in-memory stores instead."
        )


def compose(*args):
    require_docker()
    return subprocess.run(
        ["docker", "compose", *args], cwd=ROOT
    ).returncode


def dev_env():
    """Environment for locally started processes: a database when the
    compose one is reachable, otherwise the in-memory stores."""
    env = dict(os.environ)
    env.setdefault("ALLOWED_ORIGINS", DEV_ORIGINS)
    if not env.get("DATABASE_URL") and database_reachable():
        env["DATABASE_URL"] = COMPOSE_DSN
    return env


def database_reachable():
    """True when something answers on the compose database port."""
    import socket

    host, _, port = "127.0.0.1", "", "5433"
    try:
        with socket.create_connection((host, int(port)), timeout=0.4):
            return True
    except OSError:
        return False


def cmd_up(_args):
    return compose("up", "-d", "--build")


def cmd_down(args):
    extra = ["--volumes"] if args.wipe else []
    return compose("down", *extra)


def cmd_reset(_args):
    """Stop the stack and destroy its volume: a clean slate next time."""
    return compose("down", "--volumes")


def cmd_logs(_args):
    return compose("logs", "-f", "api")


def cmd_run(_args):
    require("go")
    print("running the API locally; demo data is seeded automatically")
    return subprocess.run(
        ["go", "run", "./src/cmd/server"], cwd=ROOT, env=dev_env()
    ).returncode


def cmd_seed(_args):
    require("go")
    return subprocess.run(
        ["go", "run", "./src/cmd/devseed"], cwd=ROOT, env=dev_env()
    ).returncode


def cmd_test(_args):
    require("go")
    return subprocess.run(
        ["go", "test", "./...", "-race", "-count=1"], cwd=ROOT
    ).returncode


def cmd_testdb(_args):
    require("go")
    env = dict(os.environ)
    env.setdefault("TEST_DATABASE_URL", COMPOSE_DSN)
    return subprocess.run(
        ["go", "test", "./src/tests/postgres/", "-race", "-count=1", "-v"],
        cwd=ROOT,
        env=env,
    ).returncode


def cmd_lint(_args):
    require("go")
    fmt = subprocess.run(
        ["gofmt", "-l", "."], cwd=ROOT, capture_output=True
    )
    if fmt.stdout:
        print("gofmt: unformatted files:")
        print(fmt.stdout.decode())
    vet = subprocess.run(["go", "vet", "./..."], cwd=ROOT)
    if fmt.stdout or vet.returncode != 0:
        return 1
    print("lint: ok")
    return 0


def cmd_tidy(_args):
    require("go")
    return subprocess.run(["go", "mod", "tidy"], cwd=ROOT).returncode


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)

    sub.add_parser("up", help="start the stack (db + api with demo data)")
    down = sub.add_parser("down", help="stop the stack")
    down.add_argument(
        "--volumes", action="store_true", help="also wipe the database volume"
    )
    sub.add_parser("logs", help="follow the api logs")
    sub.add_parser("reset", help="stop the stack and wipe its database")
    sub.add_parser("run", help="run the API locally")
    sub.add_parser("seed", help="create the demo data (idempotent)")
    sub.add_parser("test", help="run the test suite")
    sub.add_parser("testdb", help="run the Postgres integration suite")
    sub.add_parser("lint", help="format and vet")
    sub.add_parser("tidy", help="tidy the Go module")

    commands = {
        "up": cmd_up,
        "down": cmd_down,
        "logs": cmd_logs,
        "reset": cmd_reset,
        "run": cmd_run,
        "seed": cmd_seed,
        "test": cmd_test,
        "testdb": cmd_testdb,
        "lint": cmd_lint,
        "tidy": cmd_tidy,
    }
    args = parser.parse_args()
    sys.exit(commands[args.command](args))


if __name__ == "__main__":
    main()