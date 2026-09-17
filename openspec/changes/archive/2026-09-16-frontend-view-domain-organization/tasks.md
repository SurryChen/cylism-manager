## 1. Define the view layout

- [x] 1.1 Create applications, cluster, resources, network, and settings view directories
- [x] 1.2 Move domain pages, colocated tests, and the system settings shared stylesheet

## 2. Update dependencies

- [x] 2.1 Update router lazy-import paths for moved views
- [x] 2.2 Update relative API, component, composable, and stylesheet imports in moved views and tests
- [x] 2.3 Search for stale view paths and verify no moved file still references the old relative depth

## 3. Verify the migration

- [x] 3.1 Run the frontend test suite
- [x] 3.2 Run the Vite production build
- [x] 3.3 Run OpenSpec validation and confirm the change is ready to archive
