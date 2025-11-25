---
title: About Docs
weight: 99
---

{{% callout type="info" %}}
The documentation may not be fully up to date. Please refer to the [disclaimer]({{< ref "_index.md" >}}) for important information about the project's active development status, documentation accuracy, and ongoing efforts to stabilize the codebase.
{{% /callout %}}


This page serves as a style guide and reference for writing documentation for flwd. It outlines the conventions, formatting rules, and available components to ensure consistency across the documentation site.

## Conventions

- **Tone**: Professional, concise, and helpful.
- **Headings**: Use sentence case. H1 is reserved for the page title. Use H2 (`##`) for main sections and H3 (`###`) for subsections.
- **Links**: Use relative links with Hugo's `ref` shortcode where possible: `[link text]({{</* ref "page.md" */>}})` or `[link text]({{</* ref "page.md#anchor" */>}})`.

## Callouts

We use Hextra's callout shortcode to highlight important information. We have customized the styling to match the Catppuccin Frappé color scheme.

### Syntax

```markdown
{{%/* callout type="info" */%}}
This is an **info** callout. Used for general information.
{{%/* /callout */%}}

{{%/* callout type="warning" */%}}
This is a **warning** callout. Used for important alerts.
{{%/* /callout */%}}

{{%/* callout type="tip" */%}}
This is a **tip** callout. Used for helpful hints or success messages.
{{%/* /callout */%}}

{{%/* callout type="note" */%}}
This is a **note** callout. Used for neutral notes or sidebars.
{{%/* /callout */%}}
```

### Rendering

{{% callout type="info" %}}
**Info Callout**: This is the default type. Use it for general context, "did you know" facts, or neutral information that doesn't require immediate action but is good to know.
{{% /callout %}}

{{% callout type="warning" %}}
**Warning Callout**: Use this for critical information, potential pitfalls, breaking changes, or things the user *must* pay attention to avoid errors.
{{% /callout %}}

{{% callout type="tip" %}}
**Tip Callout**: Use this for best practices, shortcuts, success states, or "pro tips" that help the user achieve their goal more efficiently.
{{% /callout %}}

{{% callout type="note" %}}
**Note Callout**: Use this for side notes, technical details that are interesting but not critical to the main flow, or additional context.
{{% /callout %}}

## Code Blocks

We use Chroma for syntax highlighting.

### Standard Languages

Use standard language identifiers like `yaml`, `json`, `bash`, `go`, etc.

```yaml
# Example YAML
apiVersion: v1
kind: Pod
metadata:
  name: flwd-pod
```

```bash
# Example Bash
flwd run job.yaml
```

### HTTP / API Examples

We have custom styling for `http` code blocks to improve readability (light text on dark background).

**Syntax:**

````markdown
```http
GET /api/v1/runs
```
````

**Rendering:**

```http
GET /api/v1/runs
```

```http
POST /api/v1/jobs
Content-Type: application/json

{
  "name": "example-job"
}
```

## Inline Code & Tags

Inline code is styled with a subtle border and background to look like "tags". This is useful for referencing configuration fields, file paths, or specific values.

**Syntax:**
`config.yaml`, `metadata.name`, `source`

**Rendering:**
The `source` field in `flwd.yaml` defines where jobs are loaded from.
