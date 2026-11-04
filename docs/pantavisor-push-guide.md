# Ordre des push et activation Pantavisor

## 1. Catalogue : branche de travail

Commiter les changements de `github-actions` avec un message Conventional Commits
décrivant la nouvelle fonctionnalité, puis pousser une branche de travail.
La CI qualifie les images candidates AMD64/ARM64. Les références `:unpublished`
sont uniquement un mécanisme d'amorçage interne ; elles ne doivent jamais être
consommées directement par Pantavisor.

Surveiller `check`, `lint`, les variantes des jobs `build`/`test`, les contrôles
de permissions, les archives d'intégrité, Syft/Grype et les échecs attendus des
tests négatifs. Un job vert avec une étape ignorée ne démontre pas son exécution.

Les scans locaux ont signalé des alertes `high`/`critical` dans le socle KAS
historique et dans l'ancien export Core. La politique reste bloquante. Examiner
les rapports et traiter les dépendances ou produire les preuves de revue
nécessaires ; ne pas baisser le seuil ou supprimer les métadonnées de paquets.
Le push de branche sert à la qualification, pas à déclarer une release prête.

## 2. Catalogue : publication

Après réussite des contrôles et traitement des blocages, fusionner vers `main`
et lancer la préparation manuelle du workflow `main`. Le catalogue publie les
images qualifiées, signe/vérifie leurs digests, met à jour les métadonnées puis
crée son commit/tag et sa GitHub Release. Ne pas créer un tag anticipé à la main.

Relever le **vrai tag stable publié** dans `CATALOGUE_TAG`. Aucune valeur supposée
de prochaine version n'est requise par l'activation.

## 3. Core et k0s : activation puis push

Depuis le checkout du catalogue, avec Python, `gh` et ORAS :

```sh
python3 scripts/pantavisor_consumers.py --project core \
  --catalogue-tag "$CATALOGUE_TAG" --directory ../pantavisor-core
python3 scripts/pantavisor_consumers.py --project k0s \
  --catalogue-tag "$CATALOGUE_TAG" --directory ../pantavisor-k0s
```

L'outil vérifie la release, les interfaces et les digests accessibles avant de
générer `main.yml`. Il archive les anciens workflows hors du répertoire actif,
préserve les modifications préexistantes et refuse d'écraser un workflow géré
qui a été modifié localement. Relire les diffs, committer puis pousser chacun des
deux consommateurs. Une nouvelle branche effectue la sélection complète.

Configurer pour chacun l'App de release, `RELEASE_APP_ID`,
`RELEASE_APP_PRIVATE_KEY` et le bot `RENOVATE_TOKEN`. Désactiver l'instance
Renovate hébergée si elle ferait doublon. Le premier lancement de maintenance
peut être un dry-run authentifié ; la configuration seule ne prouve pas ses droits.

Surveiller les builds KAS, l'espace/durée, les smoke tests ARM64, les scans et
les reçus. Après fusion et préparation sur main, le tag `v<VERSION>` publie les
sorties déjà qualifiées. Les références OCI et ORAS sont distinctes et signées.
La GitHub Release contient le manifeste de publication par digest.

## 4. System : activation puis push

Prendre les références **ORAS** Core/k0s des releases productrices :

```sh
python3 scripts/pantavisor_consumers.py --project system \
  --catalogue-tag "$CATALOGUE_TAG" --directory ../pantavisor-system \
  --core-reference "$CORE_COMPONENT" --k0s-reference "$K0S_COMPONENT"
```

L'outil écrit les références immuables dans `compositions/components.json`.
Configurer l'App/bot de System et, si les packages sont privés, son accès en
lecture aux packages des producteurs. Relire, committer et pousser.

Surveiller les signatures des entrées, les deux profils QEMU, les rapports
publics, puis la fusion des reçus correspondant au même build. Les clés, disques
et états privés restent hors des artefacts publics. La release conserve aussi
les références d'entrée et les empreintes des bundles qualifiés.

## Reprises

Les noms d'artefacts sont propres aux tentatives. Les transferts utilisent leurs
IDs ; System parcourt toutes les pages API pour retrouver les profils du même
build, sans sorties de matrice partagées. Le tag mémorise la tentative réussie,
l'ID exact de l'artefact qualifié et le digest de son reçu. Une tentative ultérieure
ne remplace donc pas les preuves d'une release déjà finalisée.

Une publication partielle se reprend en relançant le workflow du tag. Les pushes
OCI/ORAS par digest sont idempotents ; une GitHub Release conflictuelle est
refusée. Ne pas déplacer le tag. Les artefacts qualifiés sont conservés 30 jours ;
leur expiration provoque un échec explicite, jamais un rebuild silencieux.
