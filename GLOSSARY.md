# Glossary

**Lifecycle module**: `managedResource` in `internal/provider/lifecycle.go`. It turns a resource's declaration (schema, model and hooks) into a Terraform resource, and owns everything Terraform-specific about it. Create and Update always end in a read. A read that finds nothing removes the resource from state, and a delete that finds nothing counts as deleted. Hook errors become diagnostics in one format.

**Hook**: a function a resource declares for the lifecycle module: `create`, `read`, `update`, `delete`, and optionally `awaitReady` and `importID`. Hooks take and return the resource's model, return plain errors, and report warnings through `op.Warn`.
