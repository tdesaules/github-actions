# Plan dotfiles qualification

Runs `python3 .github/scripts/plan_ci.py` in the trusted checkout (`directory`, default `.`). Inputs and outputs follow `action.yml`. Repository-specific helpers remain in the consumer, versioned with its dotfiles.

Fedora 45 supplies the runtime tools used by this operation.
