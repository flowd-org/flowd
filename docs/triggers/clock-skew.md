---
title: "GitHub Actions triggers: clock skew and missed schedules"
weight: 40
---

# Triggers: clock skew and missed schedules

This page describes a common problem where scheduled workflows or PR-triggered jobs appear to run late or not at all — often caused by clock skew on self-hosted runners, misconfigured system time synchronization, or misunderstandings about UTC scheduling.

## Symptoms

- Scheduled workflows run later than expected or not at all
- PRs created near a schedule boundary do not trigger jobs
- Self-hosted runners report large clock offsets in logs

## Root causes

- NTP or system time sync not enabled or failing on the host
- The host is suspended/hibernated and resumes with incorrect time
- Misinterpretation of GitHub Actions schedule which uses UTC

## Mitigations and suggested fixes

1. Ensure NTP or systemd-timesyncd is enabled and healthy

```bash
sudo systemctl enable --now systemd-timesyncd
timedatectl status
```

2. For WSL or ephemeral dev environments, prefer using internet time sync or a CI-run agent instead of self-hosted runners

3. Configure jittered schedules if you have many jobs to reduce contention and missed windows

```yaml
on:
  schedule:
    - cron: '0 2 * * *' # runs at 02:00 UTC
```

4. Monitor clock drift with tooling (chrony, ntpstat) and alert when offset exceeds tolerance

## Examples

Example: systemd-timesyncd enablement and verification

```
sudo systemctl enable --now systemd-timesyncd
timedatectl set-ntp true
timedatectl show-timesync --property=SystemClockSynchronised
```

## Acceptance

- The document explains causes and mitigations clearly and includes actionable commands
- Peer-reviewed and linked from docs/index
