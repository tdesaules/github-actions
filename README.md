# GitHub Docker actions — socle OCI

Actions **Docker natives** (`runs.using: docker`), exécutées directement par le
runner GitHub. Aucun wrapper composite, aucun socket moteur monté, aucune
installation de Buildah/Skopeo/Syft/Grype/Cosign sur le runner consommateur.

## État du premier lot

Ce lot implémente les dix opérations OCI, cinq images finales `scratch`, le
bootstrap local, les tests et le workflow de qualification/publication bootstrap.
Les variantes `linux/amd64` et `linux/arm64` sont obligatoires pour publier : le
workflow les construit et les teste sur deux runners natifs.

**Les images ne sont pas encore publiées.** Les références `0.0.0-bootstrap` dans
les métadonnées sont des références d'amorçage, pas des versions consommables ni
des références autorisées pour une release. La publication produit un patch
remplaçant toutes ces références par les digests des index multiarchitectures.
La validation des tags refuse toute référence mutable.

Voir [les résultats de validation](docs/validation.md),
[l'amorçage](docs/bootstrap.md) et [les prérequis](docs/runners.md).

## Actions

| Action | Image / commande | Sortie principale |
|---|---|---|
| [`oci/build-image`](oci/build-image/README.md) | Buildah `bud`, puis `push` vers une archive | Archive OCI et SHA-256 du fichier |
| [`oci/export-image`](oci/export-image/README.md) | Skopeo `copy --all --preserve-digests` | Archive OCI |
| [`oci/import-image`](oci/import-image/README.md) | Skopeo `copy --all --preserve-digests` | Layout OCI |
| [`oci/publish-image`](oci/publish-image/README.md) | Skopeo `copy --all --preserve-digests` | Image publiée |
| [`oci/get-image-digest`](oci/get-image-digest/README.md) | Skopeo `inspect --raw` | Digest du manifeste/index |
| [`oci/merge-image-index`](oci/merge-image-index/README.md) | Buildah `manifest` | Index AMD64 + ARM64 publié |
| [`oci/new-image-signature`](oci/new-image-signature/README.md) | Cosign `sign` | Signature par digest |
| [`oci/test-image-signature`](oci/test-image-signature/README.md) | Cosign `verify` | Rapport JSON |
| [`oci/new-sbom`](oci/new-sbom/README.md) | Syft `scan` | SPDX JSON ou CycloneDX JSON |
| [`oci/test-vulnerabilities`](oci/test-vulnerabilities/README.md) | Grype `sbom:` | Rapport JSON et statut du seuil |

Chaque `action.yml` documente l'intégralité des inputs et outputs. Les arguments
sont construits sans shell et sans `eval`. Les inputs sont transmis par le runner
dans `INPUT_<NOM>` ; le même entrypoint peut être testé localement avec ces variables.

## Structure

```text
.github/workflows/main.yml  # check, lint, build, release, maintenance
oci/<verbe-nom>/action.yml  # interface publique de chaque action
oci/<propriétaire>/Containerfile + .containerignore
shared/runtime/            # entrypoint Go statique, tests, collecte des dépendances
scripts/                   # orchestration Python, découverte et publication
tests/unit/                # sélection, versions, agrégation, reprise
taskfile.yml               # commandes locales
docs/                      # architecture, prérequis, amorçage et résultats
```

Pas de catalogue central : découverte des `action.yml` et `Containerfile`. Les
versions amont et les digests sont épinglés directement dans les Containerfiles.
Les images Skopeo, Cosign et Buildah sont partagées entre opérations utilisant le
même outil ; il n'existe pas d'image globale contenant tous les outils.

## Utilisation future après première publication

```yaml
jobs:
  build:
    strategy:
      matrix:
        include:
          - runner: ubuntu-24.04
            platform: linux/amd64
          - runner: ubuntu-24.04-arm
            platform: linux/arm64
    runs-on: ${{ matrix.runner }}
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683
      # Exemple : 1.0.0 n'existe pas encore.
      - uses: tdesaules/github-actions/oci/build-image@1.0.0
        with:
          context: .
          containerfile: Containerfile
          image: ghcr.io/organisation/application
          version: 1.2.3
          platform: ${{ matrix.platform }}
          output: artifacts/image.oci.tar
```

Les dépôts Pantavisor ne sont pas modifiés. Voir la
[procédure de migration](docs/consumers.md).

## Développement

```bash
task test-repository
task initialize-bootstrap             # architecture native du poste
task test-integration
task new-candidate-actions
```

Python 3.11+, Go 1.25+ et Task sont des outils de développement local. Go peut être
exécuté dans l'image de compilation épinglée, comme dans le workflow. Les actions
publiées ne nécessitent aucun de ces outils sur le runner consommateur.

## Suite du catalogue

Le lot OCI ne contient pas encore les actions `check`, les linters, git-cliff,
GitHub Releases, Renovate et Lychee. Leur ajout utilisera ce socle. En particulier,
le workflow actuel publie le **bootstrap** et valide les tags ; il n'implémente pas
encore la préparation automatique d'une release SemVer, le commit/tag via GitHub
App, ni la création de GitHub Releases. Aucun succès de ces étapes n'est simulé.

Les futurs contrôles `check` auront chacun leur commande et leur image. Le
contrôle TOML accompagnera l'introduction de `cliff.toml` à la racine.
