Local-only e2e validation run for the dedicated Autodev fixture repos.

This issue should be used with the local pipeline path:
- real GitLab issue
- real app repo
- local GitOps repo
- local work-order journal
- no real infra deployment

```json
{
  "approval": {
    "label": "delivery/approved"
  },
    "work_order": {
      "id": "wo-e2e-fixture-baseline",
      "requested_outcome": "Validate the dedicated Autodev e2e fixture app through the full local-only lane up to the dev approval checkpoint.",
      "policy_profile": "e2e-local",
    "pipeline_template": "default-v1",
    "translation": {
      "translator": "manual-e2e-template",
      "version": "v1",
      "status": "translated",
      "warnings": []
    },
      "delivery": {
        "name": "autodev-e2e-app",
        "primary_component": "api",
        "selected_components": [
          "api"
        ],
        "deploy_as_unit": true,
        "documentation": {
          "required": false,
          "docs_component": "docs",
          "required_kinds": []
        },
        "journal": {
          "name": "autodev-e2e-work-orders",
          "repo": {
            "project_path": "mph-tech/autodev-e2e-work-orders",
            "default_branch": "main",
            "working_branch_prefix": "autodev",
            "ref": "ef6d76716dedd4364d9177d3752e797603a0f89e",
            "materialization_path": "data/repos/{run_id}/journal/autodev-e2e-work-orders"
          },
          "path": "runs",
          "strategy": "git",
        "description": "Durable local journal for e2e fixture validation."
      },
      "components": {
        "api": {
          "name": "autodev-e2e-app",
          "kind": "api",
          "deployable": true,
          "repo": {
            "project_path": "mph-tech/autodev-e2e-app",
            "default_branch": "main",
            "working_branch_prefix": "autodev",
            "ref": "326d08918bd0b992ec890cae1891e1d1635ac467",
            "materialization_path": "data/repos/{run_id}/components/api"
          },
          "ownership": [
            {
              "identity": "agent",
              "paths": [
                "cmd/**",
                "internal/**",
                "docs/**",
                "config/**"
              ],
              "mutable": true
            },
            {
              "identity": "generator",
              "paths": [
                "generated/**"
              ],
              "mutable": true
            },
            {
              "identity": "governed",
              "paths": [
                "prompts/**",
                "stage-specs/**",
                "policy/**"
              ],
              "mutable": true
            }
          ],
          "release": {
            "application": {
              "artifact_name": "autodev-e2e-app",
              "image_repo": "registry.local/autodev-e2e-app"
            }
          }
        }
      },
      "environments": {
        "local": {
          "name": "local",
          "gitops_repo": {
            "project_path": "mph-tech/autodev-e2e-gitops",
            "environment": "local",
            "path": "clusters/local/autodev-e2e-app",
            "promotion_branch": "main",
            "cluster": "kind-local",
            "ref": "c138457cc86cb71e486251f695d5871802c7623a",
            "materialization_path": "data/repos/{run_id}/gitops/local"
          },
          "approval_required": false,
          "rollout_strategy": "recreate"
        },
        "dev": {
          "name": "dev",
          "gitops_repo": {
            "project_path": "mph-tech/autodev-e2e-gitops",
            "environment": "dev",
            "path": "clusters/dev/autodev-e2e-app",
            "promotion_branch": "main",
            "cluster": "dev-local",
            "ref": "c138457cc86cb71e486251f695d5871802c7623a",
            "materialization_path": "data/repos/{run_id}/gitops/dev"
          },
          "approval_required": false,
          "rollout_strategy": "rolling"
        },
        "prod": {
          "name": "prod",
          "gitops_repo": {
            "project_path": "mph-tech/autodev-e2e-gitops",
            "environment": "prod",
            "path": "clusters/prod/autodev-e2e-app",
            "promotion_branch": "main",
            "cluster": "prod-local",
            "ref": "c138457cc86cb71e486251f695d5871802c7623a",
            "materialization_path": "data/repos/{run_id}/gitops/prod"
          },
          "approval_required": true,
          "rollout_strategy": "rolling"
        }
      },
      "release": {
        "application": {
          "artifact_name": "autodev-e2e-app",
          "image_repo": "registry.local/autodev-e2e-app"
        }
      }
    }
  },
  "metadata": {
    "tenant": "e2e",
    "drift_policy": "gitops-only"
  }
}
```
