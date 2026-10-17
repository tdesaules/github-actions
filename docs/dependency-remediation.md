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

`grype-review.json` décrit cette seule analyse. `scripts/scan_review.py` la
réévalue sur chaque SBOM : tous les exemplaires du module doivent avoir des
symboles non vides et aucun ne doit contenir le package vulnérable. En l'absence
de preuve, aucune exclusion n'est produite. La règle est limitée à l'identifiant,
au namespace Go, au type de package et à sa version exacte.

`review-evidence.json` conserve les packages liés et la référence du correctif ;
`grype-reviewed.json` conserve la règle appliquée. Le résultat Grype conserve les
correspondances écartées dans `ignoredMatches`. Cette analyse ne s'applique pas
aux autres vulnérabilités du module ni aux images des consommateurs.
