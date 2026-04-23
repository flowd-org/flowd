---
title: "Quota exceeded"
---

# Quota exceeded

When a user attempts to upload artifacts larger than allowed by configuration, the system returns a quota-exceeded error. Remediation steps:

- Increase configured upload quota in registry settings.
- Implement client-side chunking and retry logic.
- Add monitoring to alert when upload rates approach limits.

Example remediation snippet:

```bash
# Client should retry with smaller chunks
curl -F "file=@part1" https://example.org/upload
```
