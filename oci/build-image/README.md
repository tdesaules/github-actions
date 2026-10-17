# build-image

Action composite lançant l'image `buildah` avec le profil Docker qualifié.
Voir [action.yml](action.yml)
pour tous les inputs/outputs et [les prérequis](../../docs/runners.md).

Construit avec `buildah bud --storage-driver=vfs --isolation=chroot`, puis exporte
dans la même invocation : le stockage interne disparaît avec l'action. `output`
désigne l'archive persistante dans le workspace. `archive-sha256` est le checksum
du fichier, pas le digest OCI.

`labels` et `build-args` sont des objets JSON de chaînes ; `secrets` est un tableau
de spécifications `id=nom,src=chemin`. Ne jamais passer de secret comme build arg.
Le contexte et les noms de fichiers peuvent contenir des espaces.

Tests : `task test-unit`, `task test-image IMAGE=buildah`,
`python3 scripts/testing.py --image buildah` et invocation publique du workflow.
