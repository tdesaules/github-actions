# Revue du catalogue après les premières PR Renovate

Révision source revue : `80bffbb` (merges Node et Go 1.27.3).
Les preuves ci-dessous distinguent les tests locaux, les images déjà publiées et
la qualification complète de cette nouvelle révision.

## Résultats vérifiés

- 47 opérations et images dédiées, avec action, Containerfile multistage terminé
  par `scratch`, exécutable Go et README. 46 actions Docker natives et l'exception
  Buildah composite documentée.
- Six jobs composés d'actions, sans step `run` dans le workflow principal. Les
  actions externes sont épinglées par commit et les images par digest.
- Les 47 index publiés référencés au moment de la revue sont accessibles sans
  authentification ; leur checksum correspond au digest et chacun expose Linux
  AMD64 et ARM64. Cette vérification concerne les pins existants, pas des candidats
  dont la publication est encore en cours.
- `go test ./...`, 28 tests Python et `python3 scripts/ci.py validate` réussis.
- Validation de 47 métadonnées d'actions, Actionlint, 47 Containerfiles, huit
  scripts shell, 64 documents Markdown, 50 fichiers YAML et deux TOML réussie.
  `tests/runner/action.yml` est une fixture de tests, pas une opération publiée.
- Revue des contrôles de publication : ensemble attendu, révision exacte,
  qualification, checksums, version candidate uniforme, signatures des plateformes
  et de l'index, refus d'un main avancé et push atomique sans déplacement de tag.
- Scans sans reconstruction et reprise entre tentatives démontrés sur GitHub,
  jusqu'à la release `1.3.1` : voir les
  [preuves détaillées](qualification-corrections.md).

Les rapports locaux sont conservés sous `/tmp/opencode/final-review-structure.json`,
`/tmp/opencode/final-review-ghcr.json` et `/tmp/opencode/final-review-lints.log`.
Ces chemins sont des preuves de travail locales, pas des assets de release.

## Corrections implémentées après la revue

Les corrections ci-dessous sont testées localement. Leur activation dépend de la
qualification native et de la publication des images modifiées ; les anciens
digests ne deviennent pas corrigés par la seule modification des sources.

### Jeton OIDC dans les arguments du processus Cosign

Au moment de la revue, `shared/publication/signature.go` et `shared/runtime/main.go` transmettaient
le jeton obtenu par le wrapper à Cosign avec `--identity-token`. Les cinq inputs
publics de mot de passe de registre sont bien transmis par environnement, mais
le contrat « secrets hors argv » n'était donc pas respecté jusqu'au processus
enfant du signataire. Aucune fuite effective n'a été constatée pendant cette revue.

La correction utilise le support de fichier de `--identity-token` dans Cosign
3.1.3 : fichier `0600`, répertoire temporaire `0700` hors workspace, suppression
après succès ou échec. Les deux chemins de signature partagent ce mécanisme.
Les tests contrôlent le contenu reçu par le signataire, les permissions,
l'absence du jeton dans les arguments enregistrés et le nettoyage après échec.
La signature OIDC réelle doit encore être requalifiée avec les images corrigées.

### Versions sources amont non extraites par le manager Renovate

`renovate.json:19` reconnaît `module@vVersion`. Les appels dans
`shared/runtime/compile-{skopeo,cosign,syft,grype}.sh:3` utilisent en revanche
`module vVersion h1:checksum` pour les sources amont. La reproduction locale
confirme que ces quatre versions ne sont pas extraites par cette expression.
Les dépendances correctives au format `module@version`, les bases, actions,
modules du dépôt et dépendances npm sont bien dans le périmètre configuré.

`renovate-tool-sources.json` complète maintenant l'extraction. Cette configuration
globale est embarquée avec le runner corrigé, afin de ne pas activer un hook absent
des anciennes images. L'extraction locale par Renovate détecte huit déclarations
sources, dont les quatre outils signalés, Buildah, Actionlint et Distribution.

Un hook fixe `/usr/local/bin/update-tool-checksums` recalcule les sommes après la
mise à jour des versions, avec `go mod download` et la vérification sum.golang.org
imposée. Seule cette commande est autorisée. Il valide l'identité du module, sa
version et le format h1, et résout toutes les sommes avant de modifier les recettes.
La configuration de dépôt conserve les gestionnaires existants. La vérification
de somme du build est conservée ; aucun checksum Git n'est substitué à la somme Go.

Tests locaux : configuration et extraction Renovate réelles, mise à jour d'une
somme h1 volontairement périmée dans une fixture conteneurisée, rejet de sommes ou
identités invalides et de l'indisponibilité du service. Un dry-run authentifié puis
une PR de mise à jour source doivent encore valider le hook dans le runner publié.

## Documentation rectifiée

La documentation d'architecture désignait encore le sélecteur Python comme
implémentation active et annonçait l'absence de shell/compilateur dans toutes les
images. Elle décrit maintenant les actions Go réellement appelées et les runtimes
nécessaires aux tests et à Renovate. Les documents de qualification ne présentent
plus les validations acquises en `1.3.1` comme des travaux futurs.

## Qualification après merges

Le run Node `41352321746` a réussi sur les deux architectures. Il n'a qualifié
que l'image Renovate ; le run du merge Go précédent avait été annulé par ce push.
La préparation complète `41352590229` a été lancée sur `80bffbb`, sans réutiliser
ce lot partiel. Son résultat, celui du tag et la maintenance sur les nouveaux
pins doivent être constatés avant de déclarer ces mises à jour publiées et validées.
