#!/bin/bash

# Template JSON
TEMPLATE='{
  "operation": "orchestrate",
  "version": "1.0.0",
  "operation_config": {
    "steps": [{"command": ["PLACEHOLDER_ENTRYPOINT"]}]
  },
  "runtime": {
    "summary": "Automated stage: PLACEHOLDER_NAME",
    "queue_mode": "auto",
    "success_criteria": {"result_status": "success"},
    "signal": {"failure_severity": "high"},
    "stats": {"model": "gpt-4", "tool_calls": 1, "tooling_usd": 0.01, "expected_artifacts": 1}
  },
  "tooling_repo": {"url": "https://gitlab.com/mph.tech/tooling.git", "ref": "main"},
  "prompt_file": "prompts/PLACEHOLDER_NAME.md",
  "container": {
    "run_as": "agent",
    "write_as": "agent",
    "permissions": {
      "writable": ["/workspace"],
      "repo_control": ["none"],
      "network": "restricted",
      "runtime_user": {"mode": "container-user", "enforcement": "required"}
    }
  },
  "allowed_secrets": [],
  "gitlab_scopes": ["api"],
  "input_schema": {},
  "output_schema": {},
  "artifact_policy": {
    "required": [],
    "retention": "30d",
    "publish_to_gitlab": false,
    "signed_summaries": false,
    "attach_manifests": false
  },
  "timeout_seconds": 300,
  "retry_policy": {"max_attempts": 3, "backoff_secs": 10},
  "max_parallelism": 1,
  "approval_required": false
}'

# Function to update a file
update_file() {
  FILE=$1
  echo "Updating $FILE..."
  
  # Get name and entrypoint
  NAME=$(jq -r '.name' "$FILE")
  ENTRYPOINT=$(jq -r '.entrypoint[0]' "$FILE")
  
  # Extract existing dependencies or default to empty
  DEPS=$(jq -c '.dependencies // []' "$FILE")
  
  # Merge the template
  jq -n --arg name "$NAME" --arg entrypoint "$ENTRYPOINT" --argjson deps "$DEPS" \
    --argjson tpl "$TEMPLATE" \
    '$tpl * {name: $name, entrypoint: [$entrypoint], dependencies: $deps} | .prompt_file |= sub("PLACEHOLDER_NAME"; $name) | .runtime.summary |= sub("PLACEHOLDER_NAME"; $name) | .operation_config.steps[0].command = [$entrypoint]' \
    > "$FILE.tmp" && mv "$FILE.tmp" "$FILE"
}

# Process all files in the directories
for file in PROD/stage-specs/*.json TEST/stage-specs/*.json; do
  # Only process files that are clearly missing fields (check for "operation_config" key)
  if ! jq -e '.operation_config' "$file" > /dev/null; then
    update_file "$file"
  fi
done
