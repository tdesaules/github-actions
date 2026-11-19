# Validate Renovate configuration

Runs `/usr/local/renovate/node /usr/local/renovate/dist/config-validator.js --strict renovate.json` in the trusted checkout (`directory`, default `.`). Inputs and outputs follow `action.yml`. Repository-specific helpers remain in the consumer, versioned with its dotfiles.

Fedora 45 supplies the runtime tools used by this operation.
