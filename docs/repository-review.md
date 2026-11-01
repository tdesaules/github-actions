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

## Points encore ouverts

### Jeton OIDC dans les arguments du processus Cosign

`shared/publication/signature.go:50` et `shared/runtime/main.go:369` transmettent
le jeton obtenu par le wrapper à Cosign avec `--identity-token`. Les cinq inputs
publics de mot de passe de registre sont bien transmis par environnement, mais
le contrat « secrets hors argv » n'est donc pas encore respecté jusqu'au processus
enfant du signataire. Aucune fuite effective n'a été constatée pendant cette revue.

Correction à qualifier : utiliser un mécanisme de transfert hors argv pris en
charge par la version de Cosign épinglée, avec nettoyage des données temporaires
et tests prouvant l'absence du jeton dans les arguments et les diagnostics.
Requalifier ensuite la signature OIDC réelle avant publication des signataires.

### Versions sources amont non extraites par le manager Renovate

`renovate.json:19` reconnaît `module@vVersion`. Les appels dans
`shared/runtime/compile-{skopeo,cosign,syft,grype}.sh:3` utilisent en revanche
`module vVersion h1:checksum` pour les sources amont. La reproduction locale
confirme que ces quatre versions ne sont pas extraites par cette expression.
Les dépendances correctives au format `module@version`, les bases, actions,
modules du dépôt et dépendances npm sont bien dans le périmètre configuré.

Compléter l'extraction des versions sources et leur mise à jour couplée avec la
somme Go, puis vérifier le résultat dans un dry-run authentifié. Ne pas supprimer
la vérification de somme pour permettre une mise à jour automatique de version.

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
