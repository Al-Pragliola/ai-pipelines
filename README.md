# ai-pipelines

A Kubernetes controller that watches for new GitHub Issues or Jira tickets and autonomously resolves them using AI coding agents (Claude Code) in a test-driven workflow.

## Description

ai-pipelines runs as a K8s operator. You define a `Pipeline` custom resource that describes which repo to watch, how to trigger (GitHub Issues or Jira), and what steps the AI should follow. When a new issue appears, the controller creates a `PipelineRun` and executes the pipeline steps as Kubernetes Jobs:

1. **Checkout** — clones the repo and creates a feature branch
2. **Write tests** — AI writes tests that verify the issue requirements (tests should fail)
3. **Implement** — AI implements the solution to make the tests pass
4. **Test** — runs the tests; on failure, loops back to implement (up to N retries)
5. **Push** — pushes the branch to a fork (with optional manual approval via diff preview)

For multi-repo setups, a **triage** step lets the AI read the ticket and select the most appropriate repository from a list of candidates before proceeding.

A web dashboard provides real-time visibility into pipeline runs, step logs, triage results, diff previews, and manual intervention (repo selection, push approval).

### Key features

- **GitHub and Jira triggers** with configurable polling and deduplication
- **TDD workflow** — write tests first, then implement, with retry loops
- **Multi-repo triage** — AI selects the target repo based on ticket content
- **Diff preview and approval** — review AI changes before they're pushed
- **Docker-in-Docker** — AI steps can build and run containers (for test suites that need it)
- **Security hardening** — AI pods run as non-root, drop all capabilities, and have no access to git credentials

## Getting Started

### Prerequisites

- Go 1.24.6+
- Docker or Podman (`CONTAINER_TOOL=podman make ...` to use Podman)
- kubectl + [Kind](https://kind.sigs.k8s.io/)
- [Tilt](https://tilt.dev/)
- Node.js 18+ (dashboard frontend)

### Step 1 — Credentials

`make kind-secrets` creates Kubernetes secrets from local credential files. Set these up first.

#### GitHub token (`github-token` secret)

Create a [GitHub personal access token](https://github.com/settings/tokens) with the `repo` scope (needed to read issues, create branches, and push to your fork).

Store it in one of two ways:

```sh
# Option A: file (takes precedence)
mkdir -p ~/.config/ai-pipelines
echo "ghp_your_token_here" > ~/.config/ai-pipelines/github-token

# Option B: environment variable
export GITHUB_TOKEN=$(gh auth token)
```

#### AI credentials (`ai-credentials` secret)

The AI pods run Claude. Two options:

**Vertex AI** (recommended — uses GCP Application Default Credentials):

```sh
gcloud auth application-default login
```

This writes credentials to `~/.config/gcloud/application_default_credentials.json`, which `make kind-secrets` reads and mounts into the cluster. In your Pipeline CR, set:

```yaml
ai:
  env:
    CLAUDE_CODE_USE_VERTEX: "1"
    ANTHROPIC_VERTEX_PROJECT_ID: "your-gcp-project-id"
    CLOUD_ML_REGION: "us-east5"
    GOOGLE_APPLICATION_CREDENTIALS: "/tmp/gcp-creds.json"
    GCE_METADATA_HOST: "localhost"   # only needed outside GCP VMs
```

**Direct Anthropic API** (alternative):

Place your Anthropic API key in the credentials file, or adapt the Pipeline CR to use `ANTHROPIC_API_KEY` as a K8s secret env var instead.

#### Jira token (`jira-token` secret, optional)

Only needed for Jira-triggered pipelines. Create a **classic** [Atlassian API token](https://id.atlassian.com/manage-profile/security/api-tokens) — granular tokens are not currently supported.

Store credentials in one of two ways:

```sh
# Option A: files (take precedence)
mkdir -p ~/.jira
echo "your-jira-api-token" > ~/.jira/creds
echo "your-email@example.com" > ~/.jira/email   # Jira Cloud only (Basic auth)

# Option B: environment variables
export JIRA_TOKEN=your-jira-api-token
export JIRA_EMAIL=your-email@example.com        # Jira Cloud only (Basic auth)
```

For Jira Server / Data Center (Bearer auth), only the token is needed — omit the email. `make kind-secrets` skips this entirely if neither `~/.jira/creds` nor `JIRA_TOKEN` is set.

### Step 2 — Local Pipeline CRs

The `local/` directory is gitignored and holds your real configuration. Copy the sanitized samples and fill in your values:

```sh
cp config/samples/ai_v1alpha1_pipeline.yaml local/pipeline-github.yaml
cp config/samples/ai_v1alpha1_pipeline_jira.yaml local/pipeline-jira.yaml  # optional
```

Key fields to edit in `local/pipeline-github.yaml`:

| Field | Description |
|-------|-------------|
| `spec.repo.owner` | GitHub org or user that owns the target repo |
| `spec.repo.name` | Repository name |
| `spec.repo.forkOwner` | Your GitHub username — where the branch gets pushed |
| `spec.trigger.github.owner` | Same as `spec.repo.owner` |
| `spec.trigger.github.repo` | Same as `spec.repo.name` |
| `spec.trigger.github.assignee` | GitHub username whose assigned issues trigger runs |
| `spec.ai.env.ANTHROPIC_VERTEX_PROJECT_ID` | GCP project ID (Vertex AI only) |
| `spec.ai.env.CLOUD_ML_REGION` | Vertex AI region, e.g. `us-east5` (Vertex AI only) |

### Step 3 — Create the cluster

```sh
GITHUB_TOKEN=$(gh auth token) make kind-setup
```

This runs four steps in order:

| Step | Make target | What it does |
|------|-------------|-------------|
| 1 | `kind-up` | Creates Kind cluster `ai-pipelines` using `hack/kind-config.yaml`, installs CRDs |
| 2 | `kind-load-image` | Builds `Dockerfile.claude` and loads it into Kind (no registry needed) |
| 3 | `kind-secrets` | Creates `github-token`, `ai-credentials`, and optionally `jira-token` secrets |
| 4 | `kind-apply-sample` | Applies `config/samples/ai_v1alpha1_pipeline.yaml` as a placeholder |

You can run individual steps with `make kind-up`, `make kind-secrets`, etc.

### Step 4 — Start the dev loop

```sh
tilt up
```

Tilt applies your `local/pipeline-github.yaml` and `local/pipeline-jira.yaml` to the cluster, starts the controller and dashboard, and watches for code changes. Services:

| Service | URL |
|---------|-----|
| Dashboard UI | http://localhost:5173 |
| Dashboard API | http://localhost:9090 |

Other useful commands:

```sh
make kind-reset    # delete all PipelineRuns for a clean re-run
make kind-down     # tear down the cluster entirely
```

### Pipeline CR example

```yaml
apiVersion: ai.aipipelines.io/v1alpha1
kind: Pipeline
metadata:
  name: my-pipeline
spec:
  repo:
    owner: "your-org"
    name: "your-repo"
    forkOwner: "your-username"
    secretRef:
      name: github-token

  trigger:
    github:
      owner: "your-org"
      repo: "your-repo"
      assignee: "your-username"
      pollInterval: "30s"
      secretRef:
        name: github-token

  ai:
    image: "localhost/ai-pipelines-claude:latest"
    # env, secretRef, credentialsMountPath...

  steps:
    - name: checkout
      type: git-checkout
      branchTemplate: "ai/{{.IssueNumber}}-{{.Timestamp}}"
    - name: write-tests
      type: ai
      promptTemplate: |
        # ... your prompt here
    - name: implement
      type: ai
      promptTemplate: |
        # ...
    - name: test
      type: ai
      onFailure: implement
      maxRetries: 3
      promptTemplate: |
        # ...
    - name: push
      type: git-push
      requireApproval: true
```

See `config/samples/` for complete examples including Jira triggers and multi-repo triage.

## Deployment

### Install CRDs

```sh
make install
```

### Deploy the controller

```sh
make docker-build docker-push IMG=<your-registry>/ai-pipelines:tag
make deploy IMG=<your-registry>/ai-pipelines:tag
```

### Apply your Pipeline CR

```sh
kubectl apply -f your-pipeline.yaml
```

### Uninstall

```sh
kubectl delete -k config/samples/   # delete CRs
make uninstall                       # delete CRDs
make undeploy                        # delete controller
```

## Contributing

Contributions are welcome. Please open an issue to discuss what you'd like to change before submitting a PR.

```sh
make test          # run unit tests (uses envtest)
make lint          # run golangci-lint
make manifests     # regenerate CRDs/RBAC after editing *_types.go
make generate      # regenerate DeepCopy methods
```

Run `make help` for the full list of targets.

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
