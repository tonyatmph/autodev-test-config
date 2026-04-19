#!/usr/bin/env python3
import argparse
import json
import pathlib
import subprocess
import sys


ROOT = pathlib.Path(__file__).resolve().parents[1]
DEFAULT_DOCKERFILE = pathlib.Path("docker/runner/Dockerfile")
IMAGE_PREFIX = "autodev-stage"


def run(cmd: list[str], cwd: pathlib.Path | None = None) -> str:
    completed = subprocess.run(
        cmd,
        cwd=str(cwd) if cwd else None,
        text=True,
        capture_output=True,
        check=False,
    )
    if completed.returncode != 0:
        raise RuntimeError(
            f"command failed ({' '.join(cmd)}):\nSTDOUT:\n{completed.stdout}\nSTDERR:\n{completed.stderr}"
        )
    return completed.stdout.strip()


def config_source_path(root: pathlib.Path, source: str) -> pathlib.Path:
    if source not in {"PROD", "TEST"}:
        raise RuntimeError(f"config source must be PROD or TEST, got {source!r}")
    return root / source


def stage_specs_dir(root: pathlib.Path, source: str) -> pathlib.Path:
    return config_source_path(root, source) / "stage-specs"


def stage_names(root: pathlib.Path, source: str) -> list[str]:
    names = []
    for path in sorted(stage_specs_dir(root, source).glob("*.json")):
        data = json.loads(path.read_text())
        name = str(data.get("name") or "").strip()
        if not name:
            raise RuntimeError(f"stage spec {path} missing name")
        names.append(name)
    return names


def image_ref(stage: str) -> str:
    return f"{IMAGE_PREFIX}-{stage}:current"


def main() -> int:
    parser = argparse.ArgumentParser(description="Build the universal runtime substrate and all stage images from an explicit config source.")
    parser.add_argument("--config-source", required=True, choices=["PROD", "TEST"])
    parser.add_argument("--push", action="store_true")
    parser.add_argument("--report", default="", help="optional path for a non-runtime build report")
    args = parser.parse_args()

    config_source = args.config_source
    base_ref = image_ref("base")
    dockerfile = ROOT / DEFAULT_DOCKERFILE
    run(
        [
            "docker",
            "build",
            "--build-arg",
            f"CONFIG_SOURCE={config_source}",
            "-t",
            base_ref,
            "-f",
            str(dockerfile),
            str(ROOT),
        ]
    )

    built: dict[str, str] = {}
    for stage in stage_names(ROOT, config_source):
        ref = image_ref(stage)
        run(["docker", "tag", base_ref, ref])
        if args.push:
            run(["docker", "push", ref])
        built[stage] = ref

    if args.report:
        report_path = pathlib.Path(args.report)
        report_path.parent.mkdir(parents=True, exist_ok=True)
        report_path.write_text(json.dumps({"config_source": config_source, "images": built}, indent=2) + "\n")
    print(config_source)
    return 0


if __name__ == "__main__":
    sys.exit(main())
