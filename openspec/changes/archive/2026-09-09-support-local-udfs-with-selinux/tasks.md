## 1. Nano SELinux Compatibility

- [x] 1.1 Add the container-scoped `label=disable` security option to the shared Nano Podman launch command and verify the existing command-construction unit test exercises it.
- [x] 1.2 Update the Podman installer unit-test expectation for both security options and verify `go test -tags=containers_image_openpgp ./internal/localinstall` passes.

## 2. Integration and Release Note

- [x] 2.1 Add a Fixed changelog entry describing restored local UDF and Virtual Schema execution on SELinux-enforcing Linux hosts and verify the changelog format check passes.
- [x] 2.2 Run the unchanged PostgreSQL Virtual Schema deployment test on an enforcing-SELinux host and verify both the pre-refresh and post-refresh rows are returned while host SELinux remains enforcing.
- [x] 2.3 Run `task fmt`, `task lint`, and `task all`, and verify the repository's complete local checks pass.
- [ ] 2.4 Re-run the manual Linux deployment workflow and verify the unchanged PostgreSQL Virtual Schema test remains green on Ubuntu.
