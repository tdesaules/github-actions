# Tests et niveaux de preuve

## Tests disponibles

- `task test-unit` : sélection, matrices, SemVer, modes, suppression/renommage,
  agrégation, refus des doublons/corruptions, reprise sans écrasement, arguments
  contenant des espaces, JSON, absence d'évaluation shell et protocole outputs.
- `task test-integration` : vrais conteneurs, smoke des cinq outils, construction
  Buildah, aller-retour archive/layout Skopeo, génération Syft, scan Grype,
  chemins contenant des espaces et propagation des erreurs.
- `task test-regression` : scénarios déterministes de sélection et publication.
- `task test-action-metadata` : parsing YAML, clés uniques, interfaces documentées
  et contrat Docker natif des dix actions.
- `task test-registry` : publication TLS locale, digests, index de deux fixtures,
  signature et vérification réelles avec une clé éphémère, sans journal public.
- `main.yml` : mêmes images candidates invoquées par de véritables actions
  Docker GitHub, sur AMD64 et ARM64 ; rapports sur chaque archive candidate.

Les copies temporaires d'actions utilisent un Dockerfile `FROM <candidate-locale>`
et non `docker://<tag-local>` : le runner GitHub tire les images `docker://`.
Les métadonnées originales ne sont pas modifiées par les tests.

Pour les moteurs Docker avec stockage classique, le chargement local passe par
une archive Docker temporaire produite avec Skopeo amont épinglé. L'ID de
configuration doit rester identique. L'archive OCI testée/scannée/publiée reste
inchangée ; cette conversion sert uniquement au transport d'exécution du runner.

Les tests locaux de Cosign couvrent le démarrage, la désactivation explicite,
le rejet des références/contraintes invalides et la signature/vérification réelles
avec une clé éphémère sur le registre TLS local. Ce dernier test utilise une
configuration sans journal public, permise uniquement avec une clé explicite.
Les tests unitaires OIDC vérifient aussi l'audience Sigstore et le bearer de
requête avec un serveur HTTPS de test. Le workflow de publication exécute le
parcours keyless réel, qualifié lors du bootstrap et de la release stable `0.1.0`.

## Base Grype

Le défaut est une mise à jour avant scan. Les rapports du pipeline enregistrent
le statut de la base utilisée. Une indisponibilité de la base ou une violation
du seuil fait échouer le scan : aucun rapport vide n'est fabriqué pour réussir.
`update-database: false` exige une base valide déjà accessible ; le répertoire
peut être configuré par `GRYPE_DB_CACHE_DIR` dans un emplacement partagé du job.
Le schedule rescanne les deux variantes publiées et actualise la base.

Le seuil est `high` par défaut. Un rapport reste exploitable après un échec de
seuil et est archivé avec `if: always()`. Des images constructibles peuvent donc
être refusées à la publication en raison de vulnérabilités.

Les tests unitaires ne remplacent ni les builds réels, ni les tests GitHub, ni
les signatures vérifiées. Voir [validation.md](validation.md) pour l'état observé.
