# Images spécialisées par action

Chaque dossier `oci/<action>` possède son `action.yml`, son `main.go` et son
`Containerfile`. L'image correspondante est
`ghcr.io/tdesaules/github-actions/oci-<action>`. Son entrypoint Go fixe l'opération
à la compilation ; il n'accepte aucun sélecteur d'opération positionnel.

Les actions Docker transmettent les inputs avec `runs.args`, sous forme
`--nom=valeur`. Seule `build-image` utilise le lanceur composite qualifié pour
Buildah, avec la même interface d'arguments. Les variables `INPUT_<NOM>` restent
utilisables localement en l'absence d'argument explicite. Les valeurs contenant
des espaces sont conservées sans interprétation par un shell.

Les archives testées sur runners natifs AMD64 et ARM64 sont publiées directement
par digest. L'index est assemblé de manière déterministe, puis publié avec
Skopeo `--preserve-digests`. Le programme vérifie les blobs, les architectures et
les digests. Les versions SemVer du catalogue désignent des tags Git et des
GitHub Releases, pas des tags OCI. Une image inchangée conserve sa référence.

## Migration

Les cinq anciens packages `buildah`, `skopeo`, `syft`, `grype` et `cosign` sont
remplacés par dix packages spécialisés. La suppression demandée des anciens
packages intervient après validation et épinglage des nouveaux. Les anciens
tags Git restent historiques : une fois leurs packages supprimés, leurs actions
ne pourront plus télécharger ces images.

Pour `merge-image-index`, fournir `docker://registre/dépôt` sans tag et récupérer
la référence immuable via la sortie `image`. La publication préalable des enfants
n'est plus obligatoire : cette action les copie avec leurs digests exacts.
Les autres interfaces d'inputs restent identiques. Les dépôts consommateurs
Pantavisor ne sont pas modifiés.

## Qualification en cours

- Tests Go, 23 tests Python, métadonnées des dix actions, actionlint et Hadolint :
  réussis localement.
- Registre TLS local : index et enfants publiés par digest, sans tag, avec
  conservation des digests.
- GHCR, signatures et exécution native des vingt variantes : à qualifier.
- Les marqueurs `:unpublished` ne sont pas des images publiées ; ils seront
  remplacés uniquement par les digests obtenus lors de cette qualification.

Les résultats historiques de `0.0.0-bootstrap` concernent la précédente
architecture à cinq images et ne valident pas cette migration.
