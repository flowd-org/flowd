---
title: "Idempotency Problems"
description: "Common idempotency failure modes, examples, and remediation steps."
---

# Idempotency Problems

This section catalogs canonical idempotency problems encountered when building services that must tolerate retries, duplicate events, or at-least-once delivery. Each problem entry has a short description, example payloads, and suggested remediation steps.

## mismatch

- Description: When the client submits semantically duplicate operations that the server treats as distinct because of insufficient deduplication keys.
- Example: POST /orders without an idempotency key.
- Remediation: Use idempotency keys, enforce deduplication on the server, and make operations idempotent.
