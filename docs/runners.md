# Runners et exécution

## Production : actions Docker natives

- Runner Linux, moteur **Docker réel** accessible par le runner GitHub.
- Runners natifs AMD64 et ARM64 ; la matrice emploie `ubuntu-24.04` et
  `ubuntu-24.04-arm`. Vérifier l'accès à ces labels pour le dépôt et son plan GitHub.
- Aucun Buildah ou Skopeo installé sur l'hôte, aucun socket monté dans les actions.
- Les actions ne lancent pas de daemon Docker interne.
- Accès réseau aux registres amont, GHCR et, selon l'opération, aux services
  Anchore/Sigstore/GitHub.

## Construction imbriquée

Buildah fonctionne avec `vfs`, `chroot` et `--network=host` **à l'intérieur du
conteneur d'action**. Ce dernier paramètre réutilise le namespace réseau du
conteneur extérieur ; l'action ne demande pas `docker run --network=host`.

Buildah utilise l'UID 0 du conteneur pour chroot et les opérations de fichiers.
Skopeo conserve aussi cet UID pour extraire les archives en préservant leur
ownership. Syft/Grype/Cosign adoptent le propriétaire de `GITHUB_OUTPUT` lorsqu'il
est non-root, afin de pouvoir écrire dans les montages du runner. Hors GitHub,
ils utilisent le propriétaire du workspace lorsque possible.

Les contraintes seccomp/AppArmor/SELinux et namespaces du moteur extérieur
peuvent interdire les opérations imbriquées. Un `action.yml` Docker n'offre pas
d'input pour ajouter `--privileged` ou `--cap-add` au lancement GitHub.
**Le fonctionnement sous Podman rootless ne prouve pas la compatibilité avec le
profil Docker d'un runner GitHub.** Le workflow doit qualifier ce dernier, avec
de vrais steps `uses:` et des constructions contenant `RUN` pendant le bootstrap.

Le bootstrap échoue explicitement si les permissions du runner sont
insuffisantes. Il n'active pas de privilèges supplémentaires automatiquement.
Un éventuel profil self-hosted devra être qualifié et documenté à partir des
échecs effectivement observés.

## Développement local

Podman rootless est supporté pour construire et tester les conteneurs :

```bash
CONTAINER_ENGINE=podman task initialize-bootstrap
```

Un poste avec Docker peut utiliser `CONTAINER_ENGINE=docker`. L'option interne
`scripts/images.py build-image --direct` est un diagnostic Podman : elle ne
qualifie ni Buildah imbriqué ni les actions Docker GitHub.

Les scripts utilisent des copies temporaires pour le build, un stockage interne
isolé par conteneur et des tags incluant run/tentative en CI. Les montages Podman
des répertoires temporaires sont relabellisés `:Z` pour SELinux. Ne pas relabelliser
globalement un home ou un dépôt partagé. Le workflow Docker ne tente aucune
modification SELinux de l'hôte.

Les fichiers produits doivent rester dans le workspace partagé ; les chemins
absolus retournés par les actions sont ceux visibles à l'intérieur de l'action.
Utiliser les inputs `output` connus pour les steps hôte suivants.
