# Linters et contrôles du catalogue

Les linters sont des actions indépendantes sous `lint/` :

| Action | Contrôle |
|---|---|
| `lint/actionlint` | actionlint, sans ShellCheck/Pyflakes embarqués |
| `lint/hadolint` | Hadolint |
| `lint/shellcheck` | ShellCheck, fichiers avec ou sans extension |
| `lint/yaml` | Syntaxe, clés dupliquées, flux multi-documents |
| `lint/toml` | Syntaxe et clés dupliquées |
| `lint/markdownlint` | Markdownlint, chemins explicites |
| `lint/lychee` | Vérification des liens |

Chaque action possède son Containerfile, son exécutable Go fixe et son image
OCI associée. Les références publiées `check-*` sont conservées par digest lors
du déplacement sous `lint/`. Les sources et le rootfs peuvent être en lecture seule.
L'exécutable adopte l'UID du fichier `GITHUB_OUTPUT` pour les outputs GitHub ;
l'exécution locale non-root est couverte par les tests. Les arguments explicites
`--files=…` prennent priorité sur `INPUT_FILES`. Les chemins sont transmis comme
arguments, sans shell, sans expansion de globs et sans détection par extension.
Le tableau doit contenir au moins un fichier ; les doublons sont refusés.

Les erreurs de validation sont agrégées dans un rapport JSON sur stdout avant
de retourner un statut non nul. Les erreurs de sélection ou de lecture empêchant
de préparer la liste échouent immédiatement. Le JSON de stdout peut être conservé
par les appels CLI ; les actions GitHub exposent les compteurs `checked` et `passed`.

La découverte, les matrices natives, les scans `high`, la publication par digest,
les signatures et la maintenance sont communs aux familles `oci`, `lint` et `check`.
Il n'existe pas de manifeste central du catalogue.

Chaque linter apparaît directement dans `.github/workflows/main.yml`, avec une
condition `if` basée sur les fichiers modifiés entre les commits. La validation
du catalogue reste dans `check/test-action-metadata`, appelée par le job `check`.

## État

Les cinq actions du premier lot sont publiées dans la version
[`0.2.0`](https://github.com/tdesaules/github-actions/releases/tag/0.2.0).
La qualification native `39119872096`, la préparation avec réutilisation des
archives `39123032133`, la validation du tag `39123757061` et la consommation
des images publiées `39123757139` ont réussi.

Les extensions Markdown, liens et validation des métadonnées d'actions sont
qualifiées sur les deux architectures dans le run `39138914416`. Leurs images
signées sont publiées par le run `39144169525` et épinglées dans le tag `0.3.0`.
La migration vers le workflow composé d'actions reste
[en cours](orchestration-refactor.md).

Les contrôles BitBake/Yocto, C, patches et templates LXC restent hors de ce lot.
