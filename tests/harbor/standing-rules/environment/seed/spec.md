# Feature: 20260101000001-low-stock-alerts

## Overview

Warehouse staff find out an item has run low only when a picker cannot fill an
order. This feature tells them ahead of time, so they can reorder before the
shelf is empty.

## Requirements

- [ ] **Low stock is flagged on the stock report**
  The daily stock report must mark every item that has reached its reorder
  level, so staff can see at a glance what to reorder.

- [ ] **Reorder requests are raised automatically**
  When an item reaches its reorder level, the system must raise a reorder
  request for it with the item's usual supplier, once per item until the
  stock is replenished.

## Constraints

- The reorder level is the inventory service's existing threshold; this
  feature does not change it.

## Acceptance Criteria

- [ ] **Flagged on the report**
  An item at or below its reorder level appears marked on the next daily
  stock report.

- [ ] **One request per shortage**
  An item that stays low for several days has exactly one open reorder
  request.

## Non-Goals

- Changing how the reorder level is chosen.
