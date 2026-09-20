#!/usr/bin/env python3
"""Render expected containers and resource thresholds for node_exporter."""

import argparse
import json
import math
import os
import sys
import tempfile
from pathlib import Path

RESOURCE_THRESHOLD_SPECS = (
    ("cpu", "warning", "OAKESTRA_RESOURCE_CPU_WARNING_PERCENT", 80.0),
    ("cpu", "critical", "OAKESTRA_RESOURCE_CPU_CRITICAL_PERCENT", 90.0),
    (
        "memory_available",
        "warning",
        "OAKESTRA_RESOURCE_MEMORY_WARNING_AVAILABLE_PERCENT",
        15.0,
    ),
    (
        "memory_available",
        "critical",
        "OAKESTRA_RESOURCE_MEMORY_CRITICAL_AVAILABLE_PERCENT",
        10.0,
    ),
    ("disk_free", "warning", "OAKESTRA_RESOURCE_DISK_WARNING_FREE_PERCENT", 15.0),
    ("disk_free", "critical", "OAKESTRA_RESOURCE_DISK_CRITICAL_FREE_PERCENT", 10.0),
)


def quoted(value):
    return '"' + value.replace("\\", "\\\\").replace("\n", "\\n").replace('"', '\\"') + '"'


def parse_resource_thresholds(environ=None):
    environ = os.environ if environ is None else environ
    thresholds = {}
    for resource, severity, variable, default in RESOURCE_THRESHOLD_SPECS:
        raw_value = environ.get(variable, default)
        try:
            value = float(raw_value)
        except (TypeError, ValueError) as error:
            raise ValueError(f"{variable} must be a numeric percentage") from error
        if not math.isfinite(value) or not 0 <= value <= 100:
            raise ValueError(f"{variable} must be between 0 and 100")
        thresholds[(resource, severity)] = value

    if thresholds[("cpu", "warning")] >= thresholds[("cpu", "critical")]:
        raise ValueError("CPU warning threshold must be lower than CPU critical threshold")
    for resource in ("memory_available", "disk_free"):
        if thresholds[(resource, "warning")] <= thresholds[(resource, "critical")]:
            raise ValueError(
                f"{resource} warning threshold must be higher than its critical threshold"
            )
    return thresholds


def render_inventory(config, active_profiles=(), resource_thresholds=None):
    if not isinstance(config, dict):
        raise TypeError("Compose configuration must be an object")
    project = config.get("name")
    if not isinstance(project, str) or not project:
        raise ValueError("Compose configuration must have a project name")
    services = config.get("services")
    if not isinstance(services, dict):
        raise TypeError("Compose configuration must contain services")
    active_profiles = set(active_profiles)
    lines = [
        "# HELP oakestra_expected_container_replicas Desired replicas from resolved Compose configuration.",
        "# TYPE oakestra_expected_container_replicas gauge",
    ]
    count = 0
    for name, service in sorted(services.items()):
        if not isinstance(service, dict):
            raise TypeError(f"Service {name} must be an object")
        profiles = set(service.get("profiles", []))
        if profiles and "*" not in active_profiles and not profiles.intersection(active_profiles):
            continue
        labels = service.get("labels", {})
        if not isinstance(labels, dict):
            raise TypeError("Use resolved Compose JSON, with labels represented as an object")
        if str(labels.get("oakestra.monitoring.ignore", "false")).lower() == "true":
            continue
        if labels.get("oakestra.logging.collector") not in {"root", "cluster", "one-doc"}:
            continue
        cluster_id = labels.get("oakestra.cluster.id")
        if not isinstance(cluster_id, str) or not cluster_id:
            raise ValueError(f"Service {name} has no oakestra.cluster.id label")
        replicas = service.get("scale", service.get("deploy", {}).get("replicas", 1))
        if isinstance(replicas, bool) or not isinstance(replicas, int):
            raise TypeError(f"Service {name} has a non-integer replica count")
        if replicas < 0:
            raise ValueError(f"Service {name} has a negative replica count")
        if not replicas:
            continue
        fields = {"cluster_id": cluster_id, "compose_service": name, "compose_project": project}
        selector = ",".join(f"{key}={quoted(value)}" for key, value in fields.items())
        lines.append(f"oakestra_expected_container_replicas{{{selector}}} {replicas}")
        count += 1
    lines.extend(
        [
            "# HELP oakestra_container_inventory_services Number of configured expected services.",
            "# TYPE oakestra_container_inventory_services gauge",
            f"oakestra_container_inventory_services{{compose_project={quoted(project)}}} {count}",
            "# HELP oakestra_resource_alert_threshold_percent Configured host resource alert threshold.",
            "# TYPE oakestra_resource_alert_threshold_percent gauge",
        ]
    )
    thresholds = resource_thresholds or parse_resource_thresholds({})
    for resource, severity, _, _ in RESOURCE_THRESHOLD_SPECS:
        value = format(thresholds[(resource, severity)], ".15g")
        lines.append(
            "oakestra_resource_alert_threshold_percent"
            f"{{resource={quoted(resource)},severity={quoted(severity)}}} {value}"
        )
    return "\n".join(lines) + "\n"


def write_inventory(destination, content):
    destination = Path(destination)
    destination.parent.mkdir(parents=True, exist_ok=True)
    temporary = None
    try:
        with tempfile.NamedTemporaryFile(
            mode="w", encoding="utf-8", dir=destination.parent, suffix=".tmp", delete=False
        ) as stream:
            temporary = Path(stream.name)
            stream.write(content)
            stream.flush()
            os.fsync(stream.fileno())
        temporary.chmod(0o644)
        os.replace(temporary, destination)
    finally:
        if temporary is not None:
            temporary.unlink(missing_ok=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--output", required=True, help="node_exporter textfile destination (.prom)"
    )
    parser.add_argument(
        "--profiles",
        default=os.environ.get("COMPOSE_PROFILES", ""),
        help="Comma-separated profiles",
    )
    args = parser.parse_args()
    try:
        config = json.load(sys.stdin)
        profiles = [profile.strip() for profile in args.profiles.split(",") if profile.strip()]
        content = render_inventory(config, profiles, parse_resource_thresholds())
        write_inventory(args.output, content)
    except (ValueError, TypeError, OSError) as error:
        parser.exit(1, f"Cannot generate container inventory: {error}\n")


if __name__ == "__main__":
    main()
