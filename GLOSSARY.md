# Glossary

**Lifecycle module**: `managedResource` in `internal/provider/lifecycle.go`. It turns a resource's declaration (schema, model and hooks) into a Terraform resource, and owns everything Terraform-specific about it. Create and Update always end in a read. A read that finds nothing removes the resource from state, and a delete that finds nothing counts as deleted. Hook errors become diagnostics in one format.

**Hook**: a function a resource declares for the lifecycle module: `create`, `read`, `update`, `delete`, and optionally `awaitReady` and `importID`. Hooks take and return the resource's model, return plain errors, and report warnings through `op.Warn`.

**Console client**: `consoleClient` in `internal/provider/console_client.go`, the only module that talks to the Camunda Administration API and the only one that knows the generated API types. It returns the provider's own types from `console_types.go`, reports missing objects as `errNotFound`, and applies the API's rules, such as refusing to change the organization owner (`errOwnerUnchangeable`).
