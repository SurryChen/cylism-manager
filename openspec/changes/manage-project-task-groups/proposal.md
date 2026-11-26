# Project task groups

## Why

Project task boards need persistent groups that users can create, rename and remove. The local frontend currently connects to a remote backend that does not expose task groups, so the previous capability check hid the creation action.

## Changes

- Store groups per product workspace and attach tasks using an optional group ID.
- Support authenticated listing, creation, renaming and deletion through the lifecycle API.
- Show the new-group action in board and list layouts. Explain the required backend update when connected to an older server.
- Preserve tasks, release associations and legacy requests during grouping changes and schema upgrades.

## Impact

The backend upgrade creates the group table and adds a non-null task group ID with a default of zero. Remote-backed local previews receive the new capability after that backend is upgraded.
