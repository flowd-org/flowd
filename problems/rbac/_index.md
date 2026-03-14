---
title: RBAC Problems
type: https://flowd.org/problems/rbac
---

# RBAC Problems

This namespace covers Role-Based Access Control errors.

## Overview

These problem types indicate authorization failures based on token scopes or explicit deny rules.

## Problem Types

* [Explicit Deny](explicit-deny.md) - 403 Forbidden due to an explicit deny rule in the token's scopes
* [No Match](no-match.md) - 404 Not Found when no RBAC rule matches the request
