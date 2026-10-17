# Amorçage et publication

## Construction locale

`task initialize-bootstrap` construit sur l'architecture native :

1. L'image Buildah amont, épinglée par digest multiarchitecture, construit notre
   image Buildah finale `scratch` et l'exporte en archive OCI.
2. L'archive est chargée et explicitement taguée par son ID de configuration.
3. Notre candidate Buildah construit les images Cosign, Grype, Skopeo et Syft.
4. Chaque outil est démarré et son architecture/entrypoint contrôlés.

Résultats : `.build/<outil>/<architecture>/image.oci.tar` et tags locaux
`localhost/github-actions/<outil>:0.0.0-bootstrap`.

Le bootstrap local n'annonce pas une validation GitHub, ni une publication
multiarchitecture si une seule architecture a été exécutée.

## Workflow

- Push de branche : contrôles, lint du socle et sélection des builds/tests.
- `workflow_dispatch` sur `main` : qualification complète des deux architectures.
- Option `publish-bootstrap` : publication après succès de tous les tests et scans.
- Option `sign` (active par défaut) : signature keyless puis vérification exacte
  de l'identité et de l'émetteur, pour les deux manifestes et l'index.
- Tag SemVer : validation des références déjà épinglées et de leurs signatures.
- Schedule : rescans avec une base Grype actualisée ; pas de build ni publication.

La publication recharge les archives déjà testées, vérifie leur checksum,
publie les variantes, assemble l'index et vérifie les digests des enfants. Aucune
image n'est reconstruite pendant cette étape.

Un artefact `action-digests.patch` est produit seulement après publication
complète. L'appliquer depuis la racine avec `git apply`, examiner puis committer
les références. Les actions restent non publiables tant que ce patch n'a pas été
appliqué. Le workflow ne pousse pas de commit/tag à ce stade.

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
- Une GitHub App n'est **pas encore nécessaire pour le bootstrap**, qui ne pousse
  pas de tag. Elle sera nécessaire pour la préparation automatique des releases
  ultérieures : permission Contents écriture, installation limitée à ce dépôt.

Aucun secret App fictif n'est déclaré. En local, la publication exige
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
la publication. Les tags déjà publiés sont réutilisés uniquement si leurs digests
correspondent. Un contenu différent est refusé ; aucun tag n'est déplacé pour
réparer un état partiel. Une reconstruction n'est pas une reprise de publication.

Le journal `publication.json` et les rapports sont archivés même en cas d'échec.
Les signatures peuvent être retentées pour les mêmes digests. Le patch n'est
généré qu'à la fin ; une action versionnée ne doit jamais référencer une image
absente. La reprise après expiration des artefacts requiert leur sauvegarde
externe, pas une reconstruction silencieuse.
