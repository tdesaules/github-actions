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

## Qualification

- Tests Go, 23 tests Python, métadonnées des dix actions, actionlint et Hadolint :
  réussis localement.
- Registre TLS local : index et enfants publiés par digest, sans tag, avec
  conservation des digests.
- Les vingt variantes passent les builds, tests d'actions natives/composite et
  scans au seuil `high` dans le [run 39049490277](https://github.com/tdesaules/github-actions/actions/runs/39049490277),
  commit `ddc6bd73fd1acef4b3426ead6203d08fd0ba6792`.
- [Publication 39051866118](https://github.com/tdesaules/github-actions/actions/runs/39051866118)
  réussie à partir des archives de ce run, sans reconstruction : dix index,
  vingt enfants, trente signatures OIDC vérifiées. Les références des actions
  sont épinglées aux digests réels.
- Vérification indépendante locale sans authentification GHCR : dix index et
  trente signatures accessibles et vérifiés avec l'identité exacte du workflow
  et l'émetteur `https://token.actions.githubusercontent.com`.

### Limite GHCR : découverte des signatures

Les images et index sont poussés par digest et ne portent aucun tag SemVer.
La publication totalement dépourvue de tags, signatures comprises, n'est pas
obtenue sur GHCR : son endpoint `/v2/<dépôt>/referrers/<digest>` renvoie 404.
Cosign utilise donc le mécanisme de repli OCI avec trois tags `sha256-<digest>`
par package, un pour chaque enfant et un pour l'index. Ces tags pointent vers
des index d'artefacts `application/vnd.dev.sigstore.bundle.v0.3+json`, pas vers
les images exécutables. Ils sont nécessaires à la découverte des signatures.
Leur suppression casserait la vérification Cosign standard. Ce comportement a
été constaté par interrogation anonyme des dix packages publiés.

Les résultats historiques de `0.0.0-bootstrap` concernent la précédente
architecture à cinq images et ne valident pas cette migration.
