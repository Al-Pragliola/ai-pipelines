# local/

This directory holds personal/environment-specific configurations for local development.
It is gitignored — nothing here is committed to the repository.

## Files

| File | Purpose |
|------|---------|
| `pipeline-github.yaml` | Pipeline CR for the GitHub-triggered pipeline (applied by Tilt) |
| `pipeline-jira.yaml` | Pipeline CR for the Jira-triggered pipeline (applied by Tilt) |

## Getting started

Copy and adapt from the sanitized examples in `config/samples/`:

```sh
cp config/samples/ai_v1alpha1_pipeline.yaml local/pipeline-github.yaml
cp config/samples/ai_v1alpha1_pipeline_jira.yaml local/pipeline-jira.yaml
```

Then fill in your values (repo owner, GCP project, Jira URL, etc.).

Tilt will automatically apply `local/pipeline-github.yaml` and `local/pipeline-jira.yaml`
to your local cluster when they change.
