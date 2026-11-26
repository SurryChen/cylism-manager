# Design

Groups belong to the product behind the project workspace. Group names are unique within that product. The API trims names and requires 1–64 characters. Ownership checks apply to every read and write, including task assignment.

Group creation and renaming use the existing lifecycle POST and PUT routes. GET task-groups returns a paginated group list, and lifecycle-options includes all group choices for the board and task editor. Deleting a group transactionally moves its tasks to group zero and increments their revisions. Release links and task contents survive deletion. Failure during reassignment rolls back the group deletion.

Existing callers that omit group_id retain the stored assignment. Explicit zero removes an assignment. Existing task revision checks reject stale edits following group deletion. Schema migration defaults legacy tasks to group zero and is safe to repeat.

The task toolbar exposes group creation in both layouts. Older backends keep a disabled action with an upgrade explanation. Newly supported workspaces show an empty ungrouped column until the user creates a group or task. The UI refreshes group choices and board columns after successful writes and keeps input after failure.
