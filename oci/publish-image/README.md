# publish-image

Action Docker et image propriétaire Skopeo. [Interface complète](action.yml).

`source` doit être une archive ou un layout OCI déjà testé. `destination` doit
commencer par `docker://`. L'action utilise `--all --preserve-digests` et échoue
si le registre impose une transformation incompatible.

Les inputs `registry`, `registry-username`, `registry-password` créent un fichier
d'authentification temporaire de mode 0600, supprimé après l'opération. Aucun
secret n'est placé dans les arguments de Skopeo.

Cette brique peut publier un tag ; la politique de non-écrasement des tags de
release est assurée par `workflow/new-publication-plan` et
`workflow/publish-catalog-images`. Utiliser ensuite
`get-image-digest` pour vérifier le digest distant.

Tests : conversion réelle d'archives/layouts, refus d'une source registre dans
le test natif, publication réelle lors du workflow bootstrap autorisé.
