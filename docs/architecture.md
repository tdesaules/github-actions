# Architecture du socle

## Images minimales

Chaque opération possède son action, son exécutable Go et son image multistage.
Privilégier `scratch` pour les outils autonomes dont les dépendances complètes
s'y prêtent. Pour un environnement de compilation nécessitant shell, Python,
Git et compilateurs, conserver une distribution maintenue et sa gestion des
paquets plutôt que reconstruire manuellement cet environnement dans scratch.
Les compilateurs sont alors des dépendances d'exécution légitimes de l'action.
Les linters sont sous `lint/`, les suites sous `check/`.

Les bases autorisées, pour tous les stages et donneurs externes, sont
`quay.io/fedora/fedora-minimal`, `quay.io/fedora/fedora`, `scratch`,
`docker.io/library/alpine`, `docker.io/chainguard/wolfi-base`,
`docker.io/redhat/ubi10-micro` et `docker.io/redhat/ubi10-minimal`.
Choisir selon compatibilité, maintenance sécurité puis taille ; une base minimale
ne garantit pas moins de CVE après installation des dépendances. Résoudre les
digests et architectures réels. Conserver les licences et métadonnées de paquets.
Les anciens donneurs hors liste restent une dette de migration signalée, pas
des bases implicitement approuvées. Le dernier stage est nommé `final`.

Les labels titre/source/licence sont présents. Les versions outil sont celles
des Containerfiles ; elles ne sont pas les versions du catalogue ou des images
d'intégration. Le bootstrap utilise `0.0.0-bootstrap`, le développement
`0.0.0-sha-<sha>-<run>`. La version candidate ne change pas entre les tentatives
d'un même run, afin de permettre la reprise partielle.

## Découverte et changements

Les actions Go `workflow/get-changed-files` et `workflow/get-image-matrix` pilotent
la sélection hébergée depuis les fichiers, métadonnées et dépendances COPY/ADD ou
montages bind du contexte. Il n'y a pas de manifeste central du catalogue. Les
scripts Python restent des outils locaux et des régressions historiques ; leur
ancienne sélection ne constitue pas le contrat du sélecteur Go actif.

- Branche existante : `before` → `after`.
- Révision `before` absente ou nulle : sélection complète.
- Commit `before` manquant après force-push : tentative de récupération du SHA
  exact avec délai borné ; sélection complète si l'ancien commit est inaccessible.
- Renommage : ancien et nouveau chemin, représentés comme suppression/ajout.
- Composant supprimé : absent de la découverte, donc pas de build ; la validation
  refuse cependant une image encore référencée sans propriétaire.
- Changement d'action : test avec l'image épinglée, sans build.
- Code partagé et LICENSE : sélection des images qui les consomment dans leurs
  recettes. Les sources variables non résolues provoquent un repli conservateur.
- Politique Grype : scans des digests publiés indépendamment des reconstructions.
- Documentation seule : matrice vide.
- Préparation : sélection depuis la release précédente ; sélection complète pour
  une première release. Maintenance sans diff.

Avant la première publication, un changement d'action seul ne peut pas tirer une
image épinglée inexistante : effectuer une qualification bootstrap complète.

La matrice couvre composant × architecture, avec `fail-fast: false` et
`max-parallel: 4`. GitHub limite une matrice à 256 jobs ; le parallélisme effectif
dépend des runners disponibles. Les résultats sont des fichiers par composant
et architecture, jamais un output matriciel partagé.

## Ajouter une opération ou un outil

1. Créer `oci/<verbe-approuvé>-<nom>/action.yml` et son README.
2. Ajouter son propre Containerfile multi-stage avec bases épinglées et étape
   finale adaptée aux besoins et nommée `final`. Son image se nomme
   `oci-<verbe-approuvé>-<nom>`.
3. Ajouter son `main.go` fixant l'opération et ses arguments autorisés, sans
   sélecteur d'opération à l'exécution ni interprétation de commande.
4. Ajouter fixtures positives/négatives et tests de scripts, d'image et d'action.
5. Ajouter un step `uses:` explicite dans `main.yml` pour le test GitHub natif :
   GitHub n'accepte pas un chemin `uses:` calculé à partir de la matrice.
6. Qualifier les deux architectures avant publication.

Les contrôles `check` possèdent chacun leur image et leur exécutable ciblé.
Les formats BitBake, Yocto, C, patches et templates LXC ne
sont pas annoncés comme lintés par le socle OCI.

## Sécurité consultative

SBOM, Grype, revues, signatures et vérifications de signatures produisent des
rapports consultatifs, y compris en cas d'erreur d'outil. `high` est une priorité
de présentation, pas un seuil d'échec du catalogue. Les résultats réels utilisent
`steps.<id>.outcome`, jamais la conclusion rendue verte par `continue-on-error`.
Une erreur ou une opération non exécutée n'est pas un résultat sain.

Builds, tests fonctionnels, identité source/plateforme, intégrité des archives et
publication restent bloquants. `tested: true` signifie qualification fonctionnelle.
`workflow/test-catalog-publication` contrôle les tags/indexes indépendamment de
Cosign. Une publication peut comporter des alertes ou ne pas être signée : le
journal et le résumé doivent le dire. Les diagnostics sécurité sont transférés
séparément des archives/reçus obligatoires.

Le pont temporaire `workflow/prepare-source-action` exécute les versions source
de la validation et du reporting tant que leurs nouvelles images ne sont pas
publiées. Il vérifie les archives et prépare des références immuables dans un
registre de job. Supprimer ses appels après publication et épinglage des vrais
digests ; ne pas confondre ce bootstrap avec l'activation des consommateurs.
