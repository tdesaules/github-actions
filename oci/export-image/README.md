# export-image

Action Docker Skopeo. [Interface complète](action.yml).

Copie un layout `oci:chemin:nom` ou une référence `docker://registre/image` vers
une archive `oci-archive:chemin.tar:nom`. Utilise `--all --preserve-digests`.
Elle n'accède pas au stockage Docker de l'hôte et ne monte aucun socket.

Exemple : `source: 'oci:layout avec espaces:fixture'`,
`destination: 'oci-archive:sortie avec espaces.tar:fixture'`.

Tests : aller-retour réel avec `import-image` dans `scripts/testing.py` et steps
natifs correspondants dans `.github/workflows/main.yml`.
