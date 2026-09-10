## 1. Archive metadata and state

- [x] 1.1 Extend custom SLC archive validation to read and return internal aliases from `build_info/language_definitions.json` without extracting the archive.
- [x] 1.2 Persist internal aliases in installed custom-SLC state and retain compatibility with existing state files.

## 2. Install and update validation

- [x] 2.1 Add generic alias-conflict checking against installed custom SLCs before a custom install is staged.
- [x] 2.2 Apply the same check to custom update while excluding the SLC record being replaced.
- [x] 2.3 Return a clear conflict error and leave deployment state and runtime unchanged when validation fails.

## 3. Tests and verification

- [x] 3.1 Add unit tests for archive alias reading, malformed metadata, multiple aliases, and case-insensitive comparison.
- [x] 3.2 Add install/update tests covering conflicts with official/custom SLCs and an update retaining its own alias.
- [x] 3.3 Run the focused Go tests and relevant deployment tests, then verify the OpenSpec scenarios.
