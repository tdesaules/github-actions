# Amorçage et publication

## Construction locale

`task initialize-bootstrap` construit sur l'architecture native :

1. L'image Buildah amont, épinglée par digest multiarchitecture, construit notre
   image `oci-build-image` finale `scratch` et l'exporte en archive OCI.
2. L'archive est chargée et explicitement taguée par son ID de configuration.
3. Notre candidate Buildah construit les autres images découvertes dans le catalogue.
4. Chaque outil est démarré et son architecture/entrypoint contrôlés.

Résultats : `.build/<image>/<architecture>/image.oci.tar` et tags locaux
`localhost/github-actions/<image>:0.0.0-bootstrap`. Ces tags servent
uniquement à l'exécution locale ; la publication GHCR utilise les digests.

Le bootstrap local n'annonce pas une validation GitHub, ni une publication
multiarchitecture si une seule architecture a été exécutée.

## Workflow

- Push de branche : contrôles, lint du socle et sélection des builds/tests.
- `workflow_dispatch` sur `main`, sans paramètres : préparation SemVer avec git-cliff ; première
  release complète, puis sélection depuis le tag stable précédent et réutilisation
  des digests inchangés. Sans changement publiable, aucune release n'est créée.
- Les signatures keyless et leurs vérifications sont rapportées à titre consultatif.
  Les tests fonctionnels, l'intégrité des artefacts et la publication restent bloquants.
- Tag SemVer : validation des références déjà épinglées et de leurs signatures,
  puis création idempotente de la GitHub Release.
- Schedule : rescans avec une base Grype actualisée, contrôle des liens et Renovate ;
  pas de build ni publication.

Pour préparer une release, ouvrir **Actions → main → Run workflow**, sélectionner
`main` et lancer. Aucun champ supplémentaire n'est nécessaire. En CLI :

```sh
gh workflow run main.yml --repo tdesaules/github-actions --ref main
```

La publication recharge les archives déjà testées, vérifie leur checksum,
publie les variantes, assemble l'index et vérifie les digests des enfants. Aucune
image n'est reconstruite pendant cette étape.

Un artefact `action-digests.patch` est produit seulement après publication
complète. L'appliquer depuis la racine avec `git apply`, examiner puis committer
les références. Les actions restent non publiables tant que ce patch n'a pas été
appliqué. En mode bootstrap, le workflow ne pousse pas de commit/tag.
En mode release SemVer, il applique le patch après publication et vérification,
génère le changelog, puis pousse atomiquement le commit et le tag via une App.
Ce chemin a publié la release stable `0.1.0` via la GitHub App :
[préparation 39112315688](https://github.com/tdesaules/github-actions/actions/runs/39112315688)
et [validation du tag 39115053239](https://github.com/tdesaules/github-actions/actions/runs/39115053239).
Les campagnes de reprise et de concurrence sont qualifiées séparément.

## Accès externes

- Activer GitHub Actions et les runners AMD64/ARM64.
- Autoriser `GITHUB_TOKEN` à publier les packages GHCR (`packages: write`).
- Donner au dépôt l'accès aux packages préexistants ; rendre les images publiques
  si elles doivent être tirées sans authentification par d'autres dépôts.
  `packages: read` ne connecte pas automatiquement Docker pour tirer l'image
  d'une action native privée. Pour les consommateurs sur runners hébergés,
  publier les images du catalogue en public ; sinon préparer l'authentification
  Docker du runner self-hosted avant le démarrage du job.
- Autoriser `id-token: write` pour signer et l'accès à Fulcio/Rekor/TUF.
- Les actions keyless utilisent explicitement `ACTIONS_ID_TOKEN_REQUEST_URL` et
  `ACTIONS_ID_TOKEN_REQUEST_TOKEN`, avec audience `sigstore`. La publication les
  transmet au conteneur et échoue s'ils sont absents.
- Une GitHub App n'est **pas nécessaire pour le bootstrap**, qui ne pousse
  pas de tag. Pour les releases SemVer et Renovate, configurer la variable
  `BOT_APP_ID` et le secret `BOT_APP_PRIVATE_KEY` de l'App bot partagée, avec
  Contents, Pull requests et Workflows en écriture, ainsi qu'Issues en écriture
  pour le Dependency Dashboard. Installer l'App sur ce dépôt. Chaque job génère
  son propre jeton d'installation limité au dépôt ; la préparation de release
  ne demande que Contents en écriture. Le token App permet au push du tag de
  déclencher sa validation, contrairement à un push avec `GITHUB_TOKEN`.

Vérifier la disponibilité de ces identifiants App dans ce dépôt. En local, la publication exige
`GHCR_TOKEN`, `GHCR_USERNAME`, les artefacts validés des deux architectures et :

```bash
task publish-bootstrap OWNER=tdesaules ARTIFACTS=/chemin/artefacts
```

Cette commande locale ne signe pas en l'absence d'un contexte OIDC. Un bootstrap
non signé ne passe pas la vérification de tag de release.

## Reprise et concurrence

Les publications GitHub sont sérialisées sans annulation. Les branches de
développement peuvent annuler leurs pipelines obsolètes.

En cas d'échec partiel, conserver les **mêmes archives et métadonnées** et relancer
la publication. Les manifestes déjà publiés sont vérifiés à leur adresse par
digest. Aucun tag OCI n'est créé ou déplacé pour réparer un état partiel.
Une reconstruction n'est pas une reprise de publication.

Le journal `publication.json` et les rapports sont archivés même en cas d'échec.
Les signatures peuvent être retentées pour les mêmes digests. Le patch n'est
généré qu'à la fin ; une action versionnée ne doit jamais référencer une image
absente. La reprise après expiration des artefacts requiert leur sauvegarde
externe, pas une reconstruction silencieuse.

La préparation publie les archives qualifiées dans son propre run. Le publisher
vérifie le commit, la version candidate, les checksums et les deux architectures.
Des artefacts absents ou ambigus font échouer la publication. Pour reprendre un
échec du job de publication sans modification de source, relancer les jobs
échoués du même run conserve les archives déjà testées. Après un correctif du
workflow, lancer une nouvelle préparation sur `main` pour utiliser ce correctif.

## Maintenance

Le job planifié réactualise la base Grype, rescane les deux architectures des
images épinglées et vérifie les liens de documentation avec Lychee conteneurisé.
Une seule base Grype actualisée est partagée entre les vingt scans ; elle n'est
pas incluse dans les artefacts de rapports.
Les politiques de revue de dépendances restent identiques à celles des builds.
`renovate.json` configure les mises à jour des bases, actions et dépendances Go.
Le job `maintenance` génère son propre jeton depuis `BOT_APP_ID` et
`BOT_APP_PRIVATE_KEY`, juste avant Renovate, et ne lance celui-ci que si la
génération réussit. Le runner reçoit ce jeton temporaire via son input `token`
et la variable interne `RENOVATE_TOKEN` ; aucun secret de ce nom ni PAT n'est requis.
L'App Renovate hébergée doit être désactivée sur ce dépôt
pour éviter les doublons. L'automerge est désactivé.
La maintenance s'exécute automatiquement le lundi à 05:23 UTC. Renovate crée
ses branches et PR lorsque des mises à jour sont proposées ; ce lancement n'est
pas un dry-run. Sa concurrence est distincte des pushes et des releases.

La migration ponctuelle depuis les cinq images partagées est terminée. L'entrée
`retire-shared-images` et les étapes de suppression ont été retirées du workflow.
Les tags Git historiques ne sont pas déplacés ; les images partagées supprimées
ne sont plus téléchargeables.
