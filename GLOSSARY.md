# Glossary

**Lifecycle module**: `managedResource` in `internal/provider/lifecycle.go`. It turns a resource's declaration (schema, model and hooks) into a Terraform resource, and owns everything Terraform-specific about it. Create and Update always end in a read. A read that finds nothing removes the resource from state, and a delete that finds nothing counts as deleted. Hook errors become diagnostics in one format.

**Hook**: a function a resource declares for the lifecycle module: `create`, `read`, `update`, `delete`, and optionally `awaitReady` and `importID`. Hooks take and return the resource's model, return plain errors, and report warnings through `op.Warn`.

**Console client**: `consoleClient` in `internal/provider/console_client.go`, the only module that talks to the Camunda Administration API and the only one that knows the generated API types. It returns the provider's own types from `console_types.go`, reports missing objects as `errNotFound`, and applies the API's rules, such as refusing to change the organization owner (`errOwnerUnchangeable`).

**Cluster child**: a resource that belongs to one cluster, such as a connector secret or a backup. It has a `cluster_id` attribute that forces replacement, is imported as `<cluster_id>/<child id>` (see `splitImportID`), and calls `WaitClusterHealthy` before changing anything, because the Console rejects changes to a cluster that is creating, updating or suspended.

**Singleton child**: a cluster child that exists at most once per cluster, such as the IP allowlist, secure connectivity or the backup schedule. Its `id` is the cluster ID, so it is imported as `<cluster_id>`; create activates or sets it and delete deactivates or clears it.

**Status wait**: `consoleClient.awaitStatus`, which polls an asynchronous Console operation (backup, activation, restore) until a target status shows up. A status that is neither pending nor a target, such as `Failed`, ends the wait with an error naming it. Resources use it from `awaitReady`, or from a hook when the wait is part of create or delete.
