---
title: Auth Problems
type: https://flowd.org/problems/auth
---

# Auth Problems

This namespace covers authentication and authorization errors.

## Overview

These problem types indicate issues with JWT tokens (invalid, expired, clock skew) or scope-based access control (missing required scopes).

## Problem Types

* [Invalid Token](invalid-token.md) - 401 Unauthorized due to invalid authentication token
* [Insufficient Scope](insufficient-scope.md) - 403 Forbidden due to missing required scope
