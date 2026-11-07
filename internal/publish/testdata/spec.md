# Feature: Nightly digest email

<!-- Guidance: describe the feature in <one paragraph>. Remove this comment. -->

## Overview

Operators want one email per day summarising what the hive filed, merged and
parked, so they can skim the night's work without opening the dashboard.

## Requirements

- [ ] **Digest schedule**: send one digest per day at the configured hour,
  in the operator's time zone.
- [ ] **Digest content**: list filed, merged and parked items with links.
- [ ] **Opt-out**: an operator can turn the digest off from the settings page.

## Acceptance Criteria

- [ ] **Digest schedule**: a digest arrives within five minutes of the hour.
- [ ] **Digest content**: every item of the last 24 hours appears once.
- [ ] **Opt-out**: no digest arrives after the operator opts out.

## Non-Goals

- Weekly or monthly digests.

## Amendments

- 2026-01-01: clarified the schedule.
