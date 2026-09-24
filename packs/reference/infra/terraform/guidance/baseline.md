## Terraform baseline

1. Resource local names are `snake_case` and describe the thing, not the type (`primary`, not `aws_instance_1`) — the type is already in the resource block's own first label.
2. Every resource that supports it carries a non-empty `tags` map, at minimum `Name` and, where the org's cost-allocation convention names one, a `team`/`cost-center` key. A module that forwards tags from a variable still needs a non-empty result at the call site.
3. Only declare resource types from an approved provider (`infra.terraform.policy`'s own rego enforces `aws_`/`random_`/`null_`/`local_` for this reference pack — extend the allowlist in a team or repo override, don't loosen it in place).
4. A module's own root has no backend block; the environment's root module configures the backend once. `terraform fmt` and `terraform validate` run in CI and locally — `render` already runs both, so a finding here means the checkout itself is wrong, not just unformatted.
5. State-touching resources (anything `create_before_destroy` protects, anything holding data) get a lifecycle block naming that explicitly — silence is not the same as intent.
6. A variable with no sane default is `nullable = false` and documented; a module's own `outputs.tf` documents every output a caller is expected to consume.
7. Prefer data sources over hardcoded ids/ARNs for anything the org already manages elsewhere (a VPC, a KMS key, an IAM role) — a literal id is a `should`-guidance violation to introduce without a comment saying why the data source didn't fit.
8. `infra.terraform.scan` (checkov) and `infra.terraform.policy` (conftest) both run against the module's own source, not a rendering — `terraform`'s own `Render` capability only initializes and validates, it does not produce a plan; a policy needing the actual planned change belongs in `dry-run`, not here.
