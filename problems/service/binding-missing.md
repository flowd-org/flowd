---
title: "binding-missing"
problem_type: "service.binding_missing"
examples:
  - description: "Example of a missing binding in service spec"
    snippet: |
      service:
        name: example
        bindings: []

This is a canonical problem leaf describing a missing binding in a service.

## Scope

Spec scope conformance: this leaf is intentionally limited to service binding-missing cases and does not describe startup, readiness, or availability failures.
