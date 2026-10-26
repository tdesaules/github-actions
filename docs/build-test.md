# Construction et qualification séparées

Le workflow possède maintenant six jobs :

```text
check → lint → build → test → release
maintenance (déclenchement séparé)
```

## Frontière des artefacts

`build` utilise l'action Buildah publiée sur le runner natif sélectionné. Il ne
qualifie pas les images. Il dépose `image.oci.tar` et `build.json` dans un artefact
`build-<image>-<architecture>-<run>`. Le reçu contient le checksum émis par le
builder, la révision, la version candidate et `tested: false`.

`test` télécharge cette archive et vérifie le reçu avant de l'exécuter. La version
candidate reste identique lors d'une relance des tests sans reconstruction.
Après les contrôles et les scans, le checksum est revérifié avant de produire
`metadata.json` avec `tested: true`. Seuls les artefacts `oci-*` issus de cette
qualification sont destinés à la publication.

Une modification de métadonnées seule teste le digest déjà publié, sans build.
Les matrices vides ignorent les deux phases. Chaque matrice reste native
AMD64/ARM64, avec `fail-fast: false` et `max-parallel: 4`. La release exige le
succès de `test`, sauf pour une sélection vide ou une réutilisation d'artefacts
préalablement validée. Le chemin de validation des tags ne reconstruit rien.

## Actions de qualification

| Action | Rôle |
| --- | --- |
| `workflow/new-build-receipt` | Reçu non qualifiant, lié au checksum du builder |
| `workflow/test-build-artifact` | Intégrité du transfert et contexte de matrice |
| `workflow/test-candidate-runtime` | Runtime natif, fixtures négatives et permissions |
| `workflow/start-test-registry` | Service de registre éphémère HTTP interne et TLS |
| `workflow/new-registry-fixtures` | CA publique et fixtures de signatures |
| `workflow/test-index-result` | Digests exacts, retry stable et absence de tags |
| `workflow/new-vulnerability-review` | Exclusions Grype fondées sur des preuves |
| `workflow/test-reviewed-vulnerabilities` | Scan `high` et métadonnées de base Grype |
| `workflow/test-step-outcomes` | Échecs attendus et absence de faux résultats |

Les actions existantes restent utilisées pour construire, transférer le candidat,
générer son `action.yml`, produire les SBOM et enregistrer sa qualification.

Le runtime de qualification matérialise les fichiers du candidat dans son propre
conteneur jetable puis exécute les tests en UID/GID 65532. Les fichiers système et
les sources appartiennent à root et sont rendus non modifiables par cet UID ;
`/tmp` et `/reports` sont les espaces d'écriture explicites. Cette protection est
fondée sur les permissions, sans montage supplémentaire. Les montages du runner
(périphériques, `/proc`, workspace et DNS) restent intacts. Les appels natifs aux
actions candidates constituent un contrôle séparé de leur interface GitHub.

Aucun socket moteur, mode privilégié ou capacité supplémentaire n'est demandé.
Le lanceur Buildah conserve son exception qualifiée. Ses fixtures comportent un
vrai `RUN` natif réussi et un `RUN` sortant en 42, sans téléchargement de base.

Le service de registre expose HTTP uniquement pour le transport candidat privé
et la CA de test. Les essais de publication/signature passent par TLS avec une
CA vérifiée. Le registre est reconstruit avec Go épinglé depuis Distribution
3.1.2 et analysé au seuil `high`. Son cycle de vie sera géré par les services du
job, sans commande Docker de démarrage ou de suppression sur le runner.

## Activation des images

La séparation des jobs est active. Les nouvelles images sont d'abord qualifiées
par ce pipeline avant leur publication immutable. Durant cette phase, la création
du reçu et une partie des tests conservent des ponts Python explicitement
identifiés. Ils seront remplacés par les actions ci-dessus après publication.
Cette phase intermédiaire ne signifie pas que `test` est déjà entièrement migré.

Le téléchargement des artefacts utilise `download-artifact` 8.0.2 épinglé par
commit. Le run `40238539314` avait produit les 94 archives qualifiées, mais la
version précédente ignorait la dernière page de l'API REST au-delà de 100
artefacts. La validation de publication a correctement rejeté le lot incomplet
(74 reçus sur 94 attendus), avant toute publication.

Les journaux et preuves JSON sont désormais écrits en `0644`, afin que l'uploader
du runner puisse lire les fichiers produits par les actions root. Les anciennes
images publiées écrivaient certains de ces fichiers en `0600`. Un environnement
Node épinglé, exécuté en root pour les actions officielles de transfert, maintient
temporairement la compatibilité des jobs `release` et `maintenance` pendant la
publication du correctif. Il sera retiré lors de l'activation des nouvelles images.
