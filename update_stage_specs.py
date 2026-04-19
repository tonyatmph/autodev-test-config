import json
import os

config_dir = "/Users/tony/.autodev/config/stage-specs/"

template = {
    "operation": "orchestrate",
    "version": "v1",
    "operation_config": { "steps": [{"command": ["/tooling/NAME/run.sh"]}] },
    "runtime": {
        "summary": "Placeholder summary",
        "success_criteria": { "result_status": "succeeded" },
        "signal": { "failure_severity": "low" },
        "stats": { "model": "none", "tool_calls": 1, "tooling_usd": 0.01, "expected_artifacts": 1 },
        "queue_mode": "auto"
    },
    "tooling_repo": { "url": "none", "ref": "main" },
    "prompt_file": "none",
    "container": {
        "run_as": "agent",
        "write_as": "agent",
        "materialize": ["workspace"],
        "permissions": {
            "writable": ["/workspace"],
            "repo_control": [],
            "network": "none",
            "runtime_user": { "mode": "shared-process", "enforcement": "advisory" }
        }
    },
    "allowed_secrets": [],
    "gitlab_scopes": [],
    "input_schema": {},
    "output_schema": {},
        "artifact_policy": {
        "required": [],
        "retention": "30d",
        "publish_to_gitlab": False,
        "signed_summaries": False,
        "attach_manifests": False
    },
    "timeout_seconds": 300,
    "retry_policy": { "max_attempts": 1, "backoff_secs": 0 },
    "max_parallelism": 1,
    "dependencies": [],
    "approval_required": False
}

for filename in os.listdir(config_dir):
    if filename.endswith(".json"):
        filepath = os.path.join(config_dir, filename)
        with open(filepath, 'r') as f:
            data = json.load(f)
        
        # Keep existing fields, fill in missing
        name = data.get("name", filename.replace(".json", ""))
        data["name"] = name
        
        # Prepare merged dict
        updated_data = template.copy()
        
        # Update command and entrypoint with the name
        updated_data["operation_config"] = { "steps": [{"command": [f"/tooling/{name}/run.sh"]}] }
        updated_data["entrypoint"] = [f"/tooling/{name}/run.sh"]
        
        # Merge existing data into updated_data
        for key, value in data.items():
            updated_data[key] = value
        
        # Add a placeholder summary if missing
        if "runtime" in updated_data and "summary" not in updated_data["runtime"]:
            updated_data["runtime"]["summary"] = f"Placeholder summary for {name}"
            
        with open(filepath, 'w') as f:
            json.dump(updated_data, f, indent=2)
        print(f"Updated {filename}")
