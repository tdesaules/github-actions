# new-image-signature

Action Docker et image propriétaire Cosign. [Interface complète](action.yml).

`image` doit être une référence `image@sha256:...`. Sans `key`, l'action demande
explicitement un token GitHub OIDC d'audience `sigstore` et le transmet à Cosign.
Le job nécessite `id-token: write` et les droits de publication au registre.

`enabled: 'false'` produit `signed=false` sans tenter d'obtenir une identité.
Une clé explicite peut être montée via le workspace ; la passphrase éventuelle
est fournie par `COSIGN_PASSWORD`, jamais par un build arg.

`transparency-log: 'false'` permet les tests/registres privés avec une clé
explicite. Ce mode est refusé en keyless. Le défaut reste la publication Sigstore.

Voir [bootstrap](../../docs/bootstrap.md) pour l'identité et les permissions.
La signature OIDC réelle n'est pas validée par un simple smoke test local.
