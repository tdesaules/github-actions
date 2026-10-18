# Reconstruction et analyse des dépendances

Les cinq outils sont reconstruits depuis les versions amont indiquées dans leurs
Containerfiles. `shared/runtime/compile-tool.sh` vérifie la somme du module source
via Go, applique les versions correctives explicites et compile avec Go 1.27.2.
Buildah utilise CGO et une liaison statique musl : son initialisation des
namespaces dépend d'un constructeur C. Les autres outils sont compilés sans CGO.
Les images finales restent `scratch`.

Les rapports conservent le SBOM SPDX, le SBOM Syft avec les symboles Go, les
métadonnées de la base Grype et le résultat JSON complet. Le seuil bloquant reste
`high`.

## Revue ciblée de GO-2026-4887

Cet avis concerne le traitement des requêtes par le plugin d'autorisation du
daemon Docker. Le [correctif amont](https://github.com/moby/moby/commit/e89edb19ad7de0407a5d31e3111cb01aa10b5a38)
modifie `pkg/authorization/authz.go`. L'avis Go affecte tout le module, sans liste
de packages ou de symboles, y compris ses clients qui n'embarquent pas ce code.

`grype-review.json` décrit cette analyse. `scripts/scan_review.py` la
réévalue sur chaque SBOM : tous les exemplaires du module doivent avoir des
symboles non vides et aucun ne doit contenir le package vulnérable. En l'absence
de preuve, aucune exclusion n'est produite. La règle est limitée à l'identifiant,
au namespace Go, au type de package et à sa version exacte.

`review-evidence.json` conserve les packages liés et la référence du correctif ;
`grype-reviewed.json` conserve la règle appliquée. Le résultat Grype conserve les
correspondances écartées dans `ignoredMatches`. Cette analyse ne s'applique pas
aux autres vulnérabilités du module ni aux images des consommateurs.

## Markdown et Renovate

L'image Markdown utilise directement `markdownlint` 0.41.1 et sa fermeture de
dépendances depuis une image amont épinglée. Le CLI et son moteur de globs,
qui embarquaient `braces`, ne sont plus inclus. Les chemins restent explicites,
la configuration est `.markdownlint.json` et aucun fichier source n'est modifié.

Renovate 44.167.1 inclut `http-cache-semantics` 4.3.0, corrigeant la version
affectée par GHSA-ch52-4w7c-c8xp. Pour
[GHSA-vfj7-8cjw-p6xm](https://github.com/micromatch/braces/issues/70), aucun
correctif publié de `braces` n'était disponible lors de cette intégration.
`maintenance/update-dependencies/patch-braces.cjs` borne donc à 64 la profondeur
des trois parcours récursifs `compile`, `expand` et `stringify`. Le build exige
les empreintes des sources amont attendues et teste le payload de 4 000 niveaux
ainsi que des expansions ordinaires.

Le nom et la version amont du package restent présents dans le SBOM. Syft
capture les empreintes des fichiers de l'image ; la revue n'écarte cet avis que
si les trois fichiers corrigés de **chaque exemplaire installé** correspondent
aux SHA-256 attendus. Toute preuve absente, ambiguë, modifiée ou d'une autre
version conserve le blocage. La correspondance et les empreintes sont conservées
dans `ignoredMatches` et `review-evidence.json`.

Les scans passent au seuil `high` : Markdown sans revue, Renovate avec cette
preuve de correction. Les deux architectures ont réussi la qualification
hébergée `39138914416` ; la publication signée `39144169525` a réutilisé ces
archives sans reconstruction.
