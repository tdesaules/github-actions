# État de la migration Pantavisor

## Audit du 3 novembre 2026

La dernière GitHub Release observée du catalogue est `1.4.3`, publiée le
2 novembre 2026. Les métadonnées locales des actions OCI et de lint référencent
leurs images par digest. La migration des consommateurs n'est pas activée.

Les contrôles GitHub en lecture seule ont établi :

- aucune entrée dans les secrets Actions de ces trois dépôts ;
- le jeton OAuth local possède `repo`, mais pas le scope `workflow` nécessaire
  pour pousser les modifications des workflows via le remote HTTPS du catalogue.

Ces observations ne démontrent pas l'installation de la GitHub App de release
sur les consommateurs. Le catalogue possède son propre secret de release ; il
n'est pas automatiquement transmis aux autres dépôts.

Le choix retenu est d'utiliser les runners GitHub hébergés. Le dépôt historique
`homelab-pantacor-arduino-ventuno-q` documente des builds AMD64 réussis sur
`ubuntu-24.04`, en environ 1 h 32 et 1 h 48 avec 84–85 Gio libres. Il prévoit
`ubuntu-24.04-arm` pour l'intégration, sans établir une réussite distante de bout
en bout. Son installation hôte de Podman et sa modification globale de sysctl
ne sont pas reprises comme contrat des nouvelles Docker Actions.

Les trois consommateurs possèdent maintenant un `kas/ci.yml` distinct de leur
configuration locale. Il limite BitBake/make à deux tâches et utilise une classe
`pantavisor-ci` pour supprimer les archives sstate après leurs hooks de création,
sans désactiver la relocalisation des sysroots natifs. Les trois configurations
fusionnées ont été vérifiées avec KAS 4.8.2 épinglé, sans checkout des dépendances.

## Actions candidates

### Sélection des travaux

`workflow/get-pantavisor-targets` est une nouvelle action Docker native avec un
exécutable Go et une image finale `scratch`. Elle produit des booléens de
sélection ; elle ne réutilise pas `workflow/get-image-matrix` et ne génère pas
de matrice dynamique. Les deux profils System restent une matrice statique.

Son `:unpublished` explicite suit le mécanisme de bootstrap du catalogue. Il ne
constitue pas une référence utilisable dans un consommateur. Les étapes ajoutées
au job `test` utilisent les métadonnées candidates générées, pas ce tag distant.

### Tests Python

Le point d'entrée de `check/test-python` accepte un répertoire de découverte
configurable, avec le défaut historique `tests/unit`. Le contrat publié reste
inchangé tant que l'image candidate n'est pas publiée. Le workflow teste aussi
l'extension via l'environnement du candidat reconstruit.

L'input facultatif `tests-directory` est déclaré dans les sources, avec défaut
`tests/unit`, et transmis explicitement à l'environnement du candidat. Les
consommateurs ne l'utiliseront qu'après qualification/publication du nouveau
digest et du tag du catalogue ; les anciens tags gardent leur comportement.

### Classification des fichiers

La classification existante reconnaît déjà les scripts sans extension, `.script`
et `Containerfile.tools`. Une régression dédiée vérifie que les recettes BitBake,
les `.conf` Yocto et les templates LXC ne sont pas classés comme Shell/YAML/TOML.

### Contrôles System et composants portables

`check/test-pantavisor-system` fournit les outils des contrôles rapides System,
dont GNU cpio et GRUB, et invoque uniquement son opération `check`.

`oci/test-component` vérifie les exports publics de schéma 2 dans un exécutable
Go indépendant des producteurs. Il vérifie inventaire, empreintes, provenance et
contrats ; il ne confond pas digest du descripteur et digest du manifeste ORAS.
Les exports Core, k0s et BSP du 16 octobre présents localement ont été vérifiés
avec cette image. Les tests négatifs couvrent corruption, liens symboliques,
fichiers inattendus, provenance incohérente et états privés.

## Validation locale effectuée

Les quatre images candidates ont été construites avec Podman rootless sur AMD64.
Les tests conteneurisés ont vérifié :

- le défaut Python historique, un chemin avec espaces et la priorité CLI ;
- la propagation d'un véritable échec de test et le refus des chemins invalides ;
- les décisions du sélecteur, les entrées JSON invalides et les fichiers supprimés ;
- la racine du conteneur en lecture seule et les permissions de `GITHUB_OUTPUT` ;
- les 10 tests Core et les 8 tests k0s dans l'image Python candidate, avec les
  dépôts montés en lecture seule et sans modifier leurs sources ;
- les 29 tests System et ses contrôles de composition, syntaxe et documentation
  dans l'action System candidate.

Commandes reproductibles depuis le catalogue :

```sh
go test ./...
python3 -B -m unittest discover -s tests/unit -v
python3 scripts/ci.py validate
podman build -f check/test-python/Containerfile \
  -t localhost/github-actions/check-test-python:pantavisor-candidate .
podman build -f workflow/get-pantavisor-targets/Containerfile \
  -t localhost/github-actions/workflow-get-pantavisor-targets:pantavisor-candidate .
python3 scripts/test-suites.py --image check-test-python \
  --reference localhost/github-actions/check-test-python:pantavisor-candidate
python3 scripts/test-pantavisor-targets.py \
  --reference localhost/github-actions/workflow-get-pantavisor-targets:pantavisor-candidate
```

Les builds locaux ne valent pas qualification Docker Actions hébergée, ni
qualification ARM64, scan, signature ou publication. Aucun nouveau tag ou digest
de consommation n'est déclaré.

## Chaîne complémentaire implémentée

Les opérations KAS, packaging rootfs/BSP, assemblage, tests rootfs/QEMU,
téléchargement/publication ORAS, reçus d'intégrité et releases sont maintenant
implémentées dans des actions spécialisées. `oci/new-rootfs-image` produit l'OCI
ARM64 depuis le tar vérifié sans moteur ni exécution croisée. ORAS 1.3.4 est
recompilé avec le Go du catalogue et une correction explicite de x/crypto.

La préparation crée un commit candidat transporté par Git bundle. Les tags
annotés mémorisent la tentative réussie et l'ID exact de l'artefact qualifié.
System exige les deux profils pour les mêmes sorties. La sélection des reçus
parcourt toutes les pages API et ne dépend pas de sorties de matrice concurrentes.

`templates/pantavisor-producer.yml` et `scripts/pantavisor_consumers.py` définissent
les workflows complets à six jobs. L'activation vérifie un vrai tag publié et ses
digests avant de générer `main.yml` et d'archiver les anciens workflows. Les
configurations Renovate, de lint et les guides d'activation sont présents dans
les trois consommateurs.

## Preuves complémentaires

- Les 25 images candidates (24 nouvelles actions et l'extension Python) ont
  toutes réussi leur construction AMD64 et leur contrôle d'exécution natif.
  Les journaux finaux sont sous `.build/pantavisor-validation/*.final-build.log`
  et `*.final-smoke.log`. Cela ne remplace pas la qualification ARM64 hébergée.
- Les deux profils ont passé les sept scénarios dans l'action QEMU locale :
  `pantavisor-system/artifacts/actions-base-v3-reports-20261103/` et
  `pantavisor-system/artifacts/actions-k0s-v3-reports-20261103/`.
- Le cycle ORAS publication par digest, reprise identique, téléchargement exact,
  signature et vérification Cosign a réussi dans un registre TLS local isolé
  (`scripts/test-component-registry.py`). Ce n'est pas un test OIDC GitHub.
- Les tests Go couvrent le commit candidat, le transfert exact, le push atomique,
  main avancé, les tags existants, les releases conflictuelles, les artefacts
  expirés et la pagination des résultats de plusieurs tentatives.
- Les scans de l'ancien export Core et du socle KAS historique restent bloquants.
  Les rapports sont respectivement dans
  `pantavisor-core/artifacts/ci-action-image/grype.json` et
  `.build/pantavisor-validation/kas-action.grype.json`. Les signalements doivent
  être traités/revus ; ils ne sont pas supprimés pour obtenir une CI verte.
- Les essais froids KAS et corrections de fermeture du runtime sont conservés
  dans `.build/pantavisor-validation/kas-core-*/` ; aucun succès froid distant
  n'est déclaré à ce stade.
- La reprise de `kas-core-v5` a réussi après génération du cache du chargeur
  dynamique utilisé par uninative. Les sorties ont été exportées avant nettoyage
  du build, puis packagées et vérifiées. L'OCI ARM64 et les smoke tests Bash,
  kubectl, k0sctl, k9s ont réussi avec ces nouvelles sorties. Les preuves sont
  `kas-core-v5/resume-build.log`, `resume-environment.json` et
  `source/artifacts/ci/build-report.json` : `cold: false` indique explicitement
  une reprise, pas une nouvelle qualification froide.

Les tests locaux du catalogue passent (`go test ./...` et 33 tests Python), ainsi
que les validations Actionlint, Hadolint, métadonnées, YAML et ShellCheck ciblées.
Les annotations des scripts consommateurs décrivent leurs entrées runtime et
leurs usages intentionnels ; elles ne désactivent pas globalement ShellCheck.

## Prérequis pour poursuivre l'activation

Le propriétaire effectuera les push des workflows avec ses accès. Qualifier les
nouvelles images sur les deux architectures via le catalogue avant activation.
Pour Yocto/QEMU, poursuivre la qualification sur les runners GitHub retenus :
mesurer espace disponible et consommation, préserver les sorties avant nettoyage
des arbres de build et vérifier le profil d'exécution Docker sans sysctl global.

Configurer séparément l'App de release et le bot Renovate pour chaque dépôt.
L'App doit pouvoir pousser les commits/tags nécessaires et déclencher la CI de
tag ; les identifiants Renovate ne sont pas ceux de la release.

Les workflows actifs des consommateurs seront remplacés uniquement après
publication des actions dont ils dépendent. Suivre le
[guide des push](pantavisor-push-guide.md) : catalogue, producteurs, puis System.
Les validations distantes et le traitement des scans bloquants restent requis.
