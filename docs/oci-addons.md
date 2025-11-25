---
title: "OCI Add‑On Sources"
weight: 7
---

{{% callout type="info" %}}
The documentation may not be fully up to date. Please refer to the [disclaimer]({{< ref "_index.md" >}}) for important information about the project's active development status, documentation accuracy, and ongoing efforts to stabilize the codebase.
{{% /callout %}}

Package jobs inside a container image so code and dependencies are reproducible
and easy to distribute.

An OCI add-on is a container image that contains a job tree following the same
layout as local sources. The image is pulled and unpacked by flwd and exposed
as a source.

## API workflow

Assuming `flwd :serve` is running:

```bash
$ curl -s -X POST http://127.0.0.1:8080/sources \
    -H 'Authorization: Bearer dev-token' \
    -H 'Content-Type: application/json' \
    -d '{
          "type":"oci",
          "name":"addon",
          "ref":"ghcr.io/example/addon:1.0.0",
          "trusted":true,
          "pull_policy":"on-add"
        }'
```

Key fields:

- `type`: must be `oci` for add-ons.
- `name`: local name for the source.
- `ref`: image reference (tag or digest).
- `trusted`: explicit opt-in; untrusted images are rejected.
- `pull_policy`: when to pull (`on-add`, `on-startup`, or similar modes).

Plan a namespaced job exposed by the add-on:

```bash
$ curl -s -X POST http://127.0.0.1:8080/plans \
    -H 'Authorization: Bearer dev-token' \
    -H 'Content-Type: application/json' \
    -d '{
          "job":"addon/hello-world",
          "args":{"name":"Alice"}
        }'
```

Run it as usual via `/runs` or the CLI, using the add-on prefix.

## Security profiles for add-ons

When extracting add-ons, flwd applies the same security profiles as for other
operations:

- `secure`: only images from allowed registries are accepted; signature
  verification is enforced; extraction happens with strict defaults.
- `permissive`: verification failures generate warnings but do not block local
  use.
- `disabled`: fewer checks; suitable only for development.

Policy decisions and failures are logged and visible in events.

## Troubleshooting

Common errors:

- `source.trust.required`: resubmit with `trusted=true` (API) or the equivalent
  flag in the CLI.
- `image.registry.not.allowed`: add the registry to the `allowed_registries`
  list in your policy configuration.
- `image.signature.required`: sign the image, switch to a more permissive mode
  for local testing, or adjust `verify_signatures`.
- `E_ADDON_MANIFEST`: the add-on manifest is missing or invalid; inspect the
  problem details and rebuild the image.
- `E_OCI`: the container runtime failed to pull or unpack the image; verify
  network access and the image reference.

OCI add-ons are a good way to ship tools that need specific runtimes or
dependencies without polluting the host.
