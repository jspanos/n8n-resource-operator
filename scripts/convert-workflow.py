#!/usr/bin/env python3
"""
Convert n8n workflow JSON files to N8nWorkflow Kubernetes CRs.

Usage:
    ./scripts/convert-workflow.py workflow.json > workflow-cr.yaml
    ./scripts/convert-workflow.py --namespace n8n workflow.json > workflow-cr.yaml
"""

import json
import sys
import argparse
import re
import yaml


def slugify(name: str) -> str:
    """Convert workflow name to a valid Kubernetes resource name."""
    # Lowercase
    slug = name.lower()
    # Replace spaces and underscores with hyphens
    slug = re.sub(r'[\s_]+', '-', slug)
    # Remove non-alphanumeric characters (except hyphens)
    slug = re.sub(r'[^a-z0-9-]', '', slug)
    # Remove consecutive hyphens
    slug = re.sub(r'-+', '-', slug)
    # Trim hyphens from ends
    slug = slug.strip('-')
    # Kubernetes names must be <= 63 characters
    return slug[:63]


def convert_workflow(workflow_json: dict, namespace: str = "n8n", active: bool = True) -> dict:
    """Convert n8n workflow JSON to N8nWorkflow CR."""

    workflow_name = workflow_json.get("name", "unnamed-workflow")

    # Build the CR
    cr = {
        "apiVersion": "n8n.slys.dev/v1alpha1",
        "kind": "N8nWorkflow",
        "metadata": {
            "name": slugify(workflow_name),
            "namespace": namespace,
        },
        "spec": {
            "active": active,
            "workflow": {
                "name": workflow_name,
            }
        }
    }

    # Copy relevant workflow fields
    if "nodes" in workflow_json:
        cr["spec"]["workflow"]["nodes"] = workflow_json["nodes"]

    if "connections" in workflow_json:
        cr["spec"]["workflow"]["connections"] = workflow_json["connections"]

    if "settings" in workflow_json:
        # Remove n8n-specific settings that shouldn't be in the CR
        settings = {k: v for k, v in workflow_json["settings"].items()
                   if k not in ["availableInMCP"]}
        if settings:
            cr["spec"]["workflow"]["settings"] = settings

    if "staticData" in workflow_json and workflow_json["staticData"]:
        cr["spec"]["workflow"]["staticData"] = workflow_json["staticData"]

    if "pinData" in workflow_json and workflow_json["pinData"]:
        cr["spec"]["workflow"]["pinData"] = workflow_json["pinData"]

    return cr


def main():
    parser = argparse.ArgumentParser(description="Convert n8n workflow JSON to Kubernetes CR")
    parser.add_argument("workflow_file", help="Path to n8n workflow JSON file")
    parser.add_argument("--namespace", "-n", default="n8n", help="Kubernetes namespace (default: n8n)")
    parser.add_argument("--inactive", action="store_true", help="Create workflow as inactive")

    args = parser.parse_args()

    # Read workflow JSON
    with open(args.workflow_file, 'r') as f:
        workflow_json = json.load(f)

    # Convert to CR
    cr = convert_workflow(workflow_json, args.namespace, not args.inactive)

    # Output as YAML
    print("---")
    yaml.dump(cr, sys.stdout, default_flow_style=False, allow_unicode=True, sort_keys=False)


if __name__ == "__main__":
    main()
