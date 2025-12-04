---
description: How to release a new version of pve-exporter
---

# Release Procedure

To ensure release notes are correctly populated in GitHub Releases, follow this procedure:

1.  **Prepare Changes**: Ensure all code changes are committed.
2.  **Tag Release**: Create an **annotated tag** with the changelog in the message body.
    *   The **Subject** (first line) will be the Release Title.
    *   The **Body** (subsequent lines) will be the Release Description/Changelog.

    ```bash
    git tag -a v1.X.X -m "v1.X.X: Release Title" -m "## Changelog" -m "- Feature 1" -m "- Fix 1"
    ```

3.  **Push Tag**: Push the tag to GitHub to trigger the release workflow.
    ```bash
    git push origin v1.X.X
    ```

**Important**: The GitHub Action is configured to extract the release notes directly from the git tag annotation. Do not rely on GitHub's auto-generated notes as they may be incomplete for direct commits.
