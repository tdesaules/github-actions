# GitHub Docker actions — socle OCI

Neuf actions **Docker natives** (`runs.using: docker`) et une action Buildah
utilisant un lanceur composite avec le profil Docker qualifié sur les runners
hébergés. Aucun socket moteur monté dans les conteneurs, aucune installation de
Buildah/Skopeo/Syft/Grype/Cosign sur le runner consommateur. Le lanceur Buildah
utilise Docker et Python 3, présents sur les runners Ubuntu GitHub.

## État du premier lot

Ce lot implémente les dix opérations OCI, dix images finales `scratch`, le
bootstrap local, les tests et le workflow de qualification/publication bootstrap.
Les variantes `linux/amd64` et `linux/arm64` sont obligatoires pour publier : le
workflow les construit et les teste sur deux runners natifs.

Les dix images spécialisées `oci-<action>` sont publiées et signées sur GHCR.
Les vingt variantes AMD64/ARM64 ont passé les tests et scans ; les actions
référencent les index publiés par digest. Voir la
[publication 39051866118](https://github.com/tdesaules/github-actions/actions/runs/39051866118).
GHCR utilise des tags techniques `sha256-…` pour découvrir les signatures
Cosign ; aucun tag OCI SemVer n'est créé.

Voir [les résultats de validation](docs/validation.md),
[l'amorçage](docs/bootstrap.md) et [les prérequis](docs/runners.md).
La [migration vers les images spécialisées](docs/action-images.md) décrit
le nouveau contrat et son état de qualification.

## Actions

| Action | Image / commande | Sortie principale |
|---|---|---|
| [`oci/build-image`](oci/build-image/README.md) | Buildah `bud`, puis `push` vers une archive | Archive OCI et SHA-256 du fichier |
| [`oci/export-image`](oci/export-image/README.md) | Skopeo `copy --all --preserve-digests` | Archive OCI |
| [`oci/import-image`](oci/import-image/README.md) | Skopeo `copy --all --preserve-digests` | Layout OCI |
| [`oci/publish-image`](oci/publish-image/README.md) | Skopeo `copy --all --preserve-digests` | Image publiée |
| [`oci/get-image-digest`](oci/get-image-digest/README.md) | Skopeo `inspect --raw` | Digest du manifeste/index |
| [`oci/merge-image-index`](oci/merge-image-index/README.md) | Assemblage Go et Skopeo `copy` | Index AMD64 + ARM64 publié par digest |
| [`oci/new-image-signature`](oci/new-image-signature/README.md) | Cosign `sign` | Signature par digest |
| [`oci/test-image-signature`](oci/test-image-signature/README.md) | Cosign `verify` | Rapport JSON |
| [`oci/new-sbom`](oci/new-sbom/README.md) | Syft `scan` | SPDX JSON ou CycloneDX JSON |
| [`oci/test-vulnerabilities`](oci/test-vulnerabilities/README.md) | Grype `sbom:` | Rapport JSON et statut du seuil |

Chaque `action.yml` documente l'intégralité des inputs et outputs. Les arguments
sont construits sans shell et sans `eval`. Chaque image exécute une opération
fixe ; les inputs sont transmis sous forme `--nom=valeur` via `runs.args` (ou le
lanceur Buildah). Priorité : argument explicite, variable `INPUT_<NOM>`, défaut
du programme. Un argument vide explicite remplace aussi la variable d'environnement.

## Structure

```text
.github/workflows/main.yml  # check, lint, build, release, maintenance
oci/<verbe-nom>/action.yml  # interface publique de chaque action
oci/<verbe-nom>/Containerfile + main.go # image et exécutable spécialisés
shared/runtime/            # utilitaires Go, tests, collecte des dépendances
scripts/                   # orchestration Python, découverte et publication
tests/unit/                # sélection, versions, agrégation, reprise
taskfile.yml               # commandes locales
docs/                      # architecture, prérequis, amorçage et résultats
```

Pas de catalogue central : découverte des `action.yml` et `Containerfile`. Les
versions amont et les digests sont épinglés directement dans les Containerfiles.
Chaque action possède son image `oci-<verbe-nom>`, même lorsque plusieurs actions
utilisent le même outil. Les images du catalogue sont publiées par digest ;
SemVer identifie les tags Git et les GitHub Releases. Les images inchangées
conservent leur digest entre versions du catalogue.

## Utilisation

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
      - uses: tdesaules/github-actions/oci/build-image@c254b423947352a98a8bdf67e35c7d8fd6595477
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
publiées ne nécessitent ni Go ni Task sur le runner consommateur. Le lanceur
Buildah utilise Python 3 fourni par les runners Ubuntu GitHub.

## Suite du catalogue

Le workflow intègre git-cliff, la préparation SemVer, le push atomique commit/tag
via GitHub App, la validation des signatures et la création des GitHub Releases.
Le push automatique nécessite encore la configuration de l'App ; le bootstrap
est publié avec `GITHUB_TOKEN`. La maintenance inclut les rescans Grype et Lychee.
La configuration Renovate est fournie ; son application doit être activée.

Les futurs contrôles `check`, absents de ce lot OCI, auront chacun leur commande
et leur image. Les configurations TOML de la racine sont contrôlées par le lint.
