# Activating the chez-moi catalog actions

The eight source actions are candidates until release preparation qualifies
their native AMD64 and ARM64 images and publishes the exact tested archives.
The Python helpers under the consumer's `.github/scripts/` remain part of its
checkout. They must come from the trusted revision being qualified.

After publication, pin the consumer's `uses:` entries to the released catalog
commit. Use these paths in `main.yml`, `release.yml` and `maintenance.yml`:

| Consumer action | Catalog action |
| --- | --- |
| `plan` | `workflow/plan-chezmoi` |
| `check-assets` | `check/test-chezmoi-assets` |
| `lint-chezmoi` | `lint/chezmoi` |
| `test-chezmoi` | `check/test-chezmoi` |
| `prepare-release` | `workflow/prepare-chezmoi-release` |
| `publish-release` | `workflow/publish-chezmoi-release` |
| `validate-renovate` | `lint/renovate` |
| `validate-maintenance` | `check/test-maintenance-event` |

Keep existing permissions, inputs, event gates and artifact handoffs. Update
the action paths emitted by `plan_ci.py` and any tests referring to local
recipes before deleting the consumer bridge. Verify a branch push, maintenance,
release preparation and tag publication on GitHub before declaring activation.
