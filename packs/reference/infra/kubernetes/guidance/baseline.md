## Kubernetes baseline

1. `metadata.name` is a lowercase RFC 1123 label (letters, digits, hyphens) under 63 characters — a name that fails this breaks at apply time, not review time, so `infra.kubernetes.policy`'s own rego catches it earlier.
2. Every workload (`Deployment`/`StatefulSet`/`DaemonSet`/`Job`/`CronJob`/`Pod`) carries `app.kubernetes.io/name`, and where the org's convention names one, `app.kubernetes.io/part-of` — these are what dashboards, cost allocation and `kubectl get -l` group by; a resource with none of them is invisible to all three.
3. No container image is untagged or `:latest` — a rendering is redeployed byte-identical or it isn't, and a floating tag makes that untrue by definition, not by accident.
4. Every container declares `resources.requests`/`resources.limits`; a workload with neither is a `should`-guidance violation to introduce without a written reason (a sidecar with genuinely unpredictable load is the usual one).
5. Prefer a `Deployment`/`StatefulSet` over a bare `Pod` for anything long-running — a bare Pod has no restart policy an operator can rely on.
6. Secrets are referenced (`envFrom`/`secretKeyRef`/a mounted volume), never inlined as a plain `env.value` — this addon's own `Render` already marks a Secret's `data`/`stringData` values sensitive at render time; a literal in a `ConfigMap` or a container's own args is invisible to that marking.
7. `infra.kubernetes.policy` (conftest) and `infra.kubernetes.scan` (trivy config) both run against the addon's own rendering (`input: render`) — a raw kustomize overlay or Helm values file is not what actually gets applied, so checking the pre-render source would miss what a base layer or a template default contributes.
