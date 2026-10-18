# Premier lot check

Le lot retenu comprend cinq actions indépendantes :

| Action | Contrôle |
|---|---|
| `check/test-github-workflow` | actionlint, sans ShellCheck/Pyflakes embarqués |
| `check/test-containerfile` | Hadolint |
| `check/test-shell` | ShellCheck, fichiers avec ou sans extension |
| `check/test-yaml` | Syntaxe, clés dupliquées, flux multi-documents |
| `check/test-toml` | Syntaxe et clés dupliquées |

Chaque action possède son Containerfile, son exécutable Go fixe et son image
`check-<action>`. Les sources et le rootfs peuvent être en lecture seule.
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
les signatures et la maintenance sont communs aux familles `oci` et `check`.
Il n'existe pas de manifeste central du catalogue.

## État

Implémentation et tests locaux en cours ; références `:unpublished` temporaires.
La qualification hébergée puis la publication signée doivent réussir avant de
recommander une version Git de ces actions aux consommateurs.

Les contrôles BitBake/Yocto, C, patches et templates LXC restent hors de ce lot.
