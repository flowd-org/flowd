---
title: "Troubleshooting"
weight: 11
---

{{% callout type="info" %}}
The documentation may not be up to date. See the [disclaimer]({{< ref "_index.md" >}}) for important information about the project's active development status, documentation accuracy, and ongoing efforts to stabilize the codebase.
{{% /callout %}}

Common problems and how to fix them.

## Container runtime and images

- `container.runtime.unavailable`  
  Podman or Docker is missing or not usable by the `flwd` process. Install a
  supported rootless runtime and ensure it is on `PATH`.

- `container.name.conflict`  
  An orphaned container is blocking reuse. Remove it manually, for example:

  ```bash
  $ podman rm -f NAME
  ```

- `E_OCI`  
  The container runtime failed to pull or start an image. Check the
  podman/docker logs and network access, and verify the image reference.

## OCI add-ons and registries

- `source.trust.required`  
  OCI sources must opt in. Set `trusted=true` when adding the source over the
  API, or use the corresponding CLI flag.

- `image.registry.not.allowed` / `source.not.allowed`  
  The registry or source is not in the allowed list. Update your policy
  configuration (`allowed_registries` or host/path allow-lists) and restart the
  server.

- `image.signature.required`  
  The image failed signature verification. Sign it (for example with `cosign`),
  relax the policy for local tests (permissive mode) or adjust
  `verify_signatures` if appropriate.

- `E_ADDON_MANIFEST`  
  The add-on manifest is missing or invalid. Inspect the problem details from
  the error response, fix the manifest and rebuild the image.

## Sources and discovery

- Jobs do not appear in `:jobs` or `/jobs`  
  Check that the source path or git URL is correct, that the job tree layout is
  valid, and that the server has permission to read those files.

- Conflicting job IDs  
  If multiple sources export the same job ID, use the `source` hint when
  planning or running jobs to disambiguate.

## Serve mode and authentication

- 401 / 403 responses  
  Verify that you are sending a valid bearer token and that it has the required
  scopes for the endpoint (`runs:write`, `jobs:read`, `sources:write`, etc.).

- CORS problems in browsers  
  In development you can use `--dev` on `:serve`. In production configure
  explicit CORS policies on your reverse proxy or in flowd’s configuration.

## Getting more information

- Run with increased logging verbosity if available.
- Use `--json` on the CLI and inspect the resulting events.
- Look at the server logs for policy decisions and errors.
- Use `flwd :tui` to quickly inspect jobs and runs without crafting HTTP
  requests.

If you hit issues that are not covered here, consider opening an issue with the
exact error message, logs and a minimal reproduction job.
