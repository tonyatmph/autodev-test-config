import json
import os

def fix_spec(file_path):
    with open(file_path, 'r') as f:
        data = json.load(f)
    
    # Add root required fields
    required_root = ["version", "tooling_repo", "prompt_file", "allowed_secrets", "gitlab_scopes", 
                     "input_schema", "output_schema", "artifact_policy", "timeout_seconds", 
                     "retry_policy", "max_parallelism", "approval_required"]
    for field in required_root:
        if field not in data:
            if field == "version": data[field] = "v1"
            elif field == "tooling_repo": data[field] = {"url": "none", "ref": "main"}
            elif field == "prompt_file": data[field] = "none"
            elif field == "allowed_secrets": data[field] = []
            elif field == "gitlab_scopes": data[field] = []
            elif field == "input_schema": data[field] = {}
            elif field == "output_schema": data[field] = {}
            elif field == "artifact_policy": data[field] = {"required": [], "retention": "30d", "publish_to_gitlab": False, "signed_summaries": False, "attach_manifests": False}
            elif field == "timeout_seconds": data[field] = 300
            elif field == "retry_policy": data[field] = {"max_attempts": 1, "backoff_secs": 0}
            elif field == "max_parallelism": data[field] = 1
            elif field == "approval_required": data[field] = False
    
    # Add container required fields
    if "container" not in data:
        data["container"] = {}
    
    data["container"].setdefault("run_as", "agent")
    data["container"].setdefault("write_as", "agent")
    data["container"].setdefault("permissions", {
        "writable": ["/workspace"],
        "repo_control": [],
        "network": "none",
        "runtime_user": {"mode": "shared-process", "enforcement": "advisory"}
    })
    
    # Add runtime required fields
    if "runtime" not in data:
        data["runtime"] = {}
        
    data["runtime"].setdefault("summary", "Placeholder")
    data["runtime"].setdefault("success_criteria", {"result_status": "succeeded"})
    data["runtime"].setdefault("signal", {"failure_severity": "low"})
    data["runtime"].setdefault("stats", {"model": "none", "tool_calls": 1, "tooling_usd": 0.01, "expected_artifacts": 1})
    data["runtime"].setdefault("queue_mode", "auto")

    with open(file_path, 'w') as f:
        json.dump(data, f, indent=2)

spec_dir = "/Users/tony/.autodev/config/stage-specs/"
for filename in os.listdir(spec_dir):
    if filename.endswith(".json"):
        fix_spec(os.path.join(spec_dir, filename))
        print(f"Fixed {filename}")

