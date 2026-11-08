# Ordre des push et activation Pantavisor

## 1. Catalogue : qualification complète sur main

Commiter les changements de `github-actions` avec un message Conventional Commits,
puis pousser `main`. La CI de push sélectionne les changements ; elle ne prouve
pas à elle seule que tout le catalogue a été qualifié. Pour construire et tester
toutes les images sur AMD64/ARM64 sans publication :

```sh
gh workflow run main.yml --repo tdesaules/github-actions --ref main \
  -f qualify-catalog=true
```

Ce mode exige `main`, force la sélection complète et expose le mode
`qualification`. Il ne calcule aucune release et ne réutilise pas d'anciens
artefacts. `release` et `maintenance` sont exclus, même si d'autres options
manuelles ont été fournies. Il est distinct de `qualify-runner`, limité aux
contrôles publiés et au lanceur Buildah. Les références `:unpublished`
sont uniquement un mécanisme d'amorçage interne ; elles ne doivent jamais être
consommées directement par Pantavisor.

Les tests peuvent écrire dans leurs registres isolés et transférer des artefacts
GitHub Actions ; ce mode ne publie pas le catalogue sur GHCR et ne crée ni commit,
tag, release GitHub ou branche Renovate.

Surveiller `check`, `lint`, les variantes des jobs `build`/`test`, les contrôles
de permissions, les archives d'intégrité, Syft/Grype et les échecs attendus des
tests négatifs. Un job vert avec une étape ignorée ne démontre pas son exécution.

Les rapports sécurité sont [consultatifs](ci-policy-transition.md), y compris
les erreurs d'outils : examiner les résultats réels, pas seulement la couleur du
job. Les tests fonctionnels, l'intégrité et la provenance restent bloquants.
Conserver les métadonnées de paquets et distinguer les opérations ignorées.

## 2. Catalogue : publication

Après qualification complète et validation des plateformes, lancer la préparation
manuelle sur le **même commit main** en réutilisant le run qualifié :

```sh
gh workflow run main.yml --repo tdesaules/github-actions --ref main \
  -f artifact-run-id="$QUALIFICATION_RUN_ID"
```

Cette préparation peut publier et écrire dans Git : ne pas la confondre avec
`qualify-catalog`. Les archives testées sont publiées sans reconstruction ;
signatures et vérifications sont tentées et rapportées. Le workflow met à jour
les digests puis crée son commit/tag ; l'exécution du tag publie la GitHub Release.
Ne pas créer un tag anticipé. Tout nouveau commit entre qualification et reprise
invalide la réutilisation de ces artefacts : consigner les résultats sans changer
la source jusqu'à la publication, ou requalifier la nouvelle révision.

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

Configurer pour chacun l'App bot partagée avec la variable `BOT_APP_ID` et le
secret `BOT_APP_PRIVATE_KEY`. Release et Renovate génèrent chacun leur jeton
d'installation limité au dépôt. Accorder à l'App Contents, Pull requests,
Workflows et Issues en écriture (Dependency Dashboard). Désactiver l'instance
Renovate hébergée si elle ferait doublon. Le premier lancement de maintenance
peut être un dry-run authentifié ; la configuration seule ne prouve pas ses droits.

Surveiller les builds KAS, l'espace/durée, les smoke tests ARM64, les scans et
les reçus. Après fusion et préparation sur main, le tag `v<VERSION>` publie les
sorties déjà qualifiées. Les références OCI et ORAS sont distinctes ; les résultats
de signature sont rapportés séparément.
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
