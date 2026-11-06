# test-vulnerabilities

Action Docker et image propriétaire Grype. [Interface complète](action.yml).

Analyse un SBOM existant. Le catalogue demande explicitement `fail-on: none` :
les résultats sont rapportés sans seuil de sortie. Pour compatibilité avec les
appelants externes, le défaut historique `high` et les valeurs `negligible`,
`low`, `medium`, `high`, `critical` restent disponibles. Le rapport reste écrit
et le code de sortie est propagé. La CI traite séparément les erreurs d'outil
comme consultatives ; elle ne configure pas de seuil pour ensuite l'ignorer.

`update-database` vaut `true` par défaut. `false` exige une base valide accessible
via `GRYPE_DB_CACHE_DIR`. Aucune base absente/périmée n'est ignorée pour rendre
le test artificiellement vert. Voir [la politique DB](../../docs/testing.md).

Le test d'intégration télécharge réellement la base et scanne le SBOM de la
fixture. Les rapports du pipeline portent sur les archives des images candidates.
