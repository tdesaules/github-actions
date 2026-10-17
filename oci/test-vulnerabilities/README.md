# test-vulnerabilities

Action Docker et image propriétaire Grype. [Interface complète](action.yml).

Analyse un SBOM existant. `fail-on` accepte `negligible`, `low`, `medium`, `high`
ou `critical` ; défaut `high`. Le rapport JSON est écrit même lorsque le seuil
est dépassé et le code de sortie de Grype est propagé.

`update-database` vaut `true` par défaut. `false` exige une base valide accessible
via `GRYPE_DB_CACHE_DIR`. Aucune base absente/périmée n'est ignorée pour rendre
le test artificiellement vert. Voir [la politique DB](../../docs/testing.md).

Le test d'intégration télécharge réellement la base et scanne le SBOM de la
fixture. Les rapports du pipeline portent sur les archives des images candidates.
