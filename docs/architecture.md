# Architecture du socle

## Images minimales

Toutes les étapes finales utilisent `FROM scratch`. Le wrapper Go est statique.
Syft, Grype et Cosign sont copiés depuis des images amont épinglées. Buildah et
Skopeo sont accompagnés de leurs bibliothèques dynamiques réellement résolues,
des certificats et configurations nécessaires. Buildah embarque aussi crun,
netavark et aardvark-dns. Aucun compilateur ni shell n'est copié dans l'image finale.

Les labels titre/source/licence sont présents. Les versions outil sont celles
des Containerfiles ; elles ne sont pas les versions du catalogue ou des images
d'intégration. Le bootstrap utilise `0.0.0-bootstrap`, le développement
`0.0.0-sha-<sha>` (avec suffixes run/tentative pour les tags locaux uniques).

## Découverte et changements

`scripts/ci.py` découvre les actions depuis leurs métadonnées et les propriétaires
d'images depuis les Containerfiles. Il n'y a ni dossier config ni manifeste central.

- Branche existante : `before` → `after`.
- Nouvelle branche : ancêtre commun avec `origin/main` ; sélection complète si
  aucun historique exploitable n'existe ou si la branche est le premier main.
- Renommage : ancien et nouveau chemin, représentés comme suppression/ajout.
- Composant supprimé : absent de la découverte, donc pas de build ; la validation
  refuse cependant une image encore référencée sans propriétaire.
- Changement d'action : test avec l'image épinglée, sans build.
- Code partagé : sélection conservatrice de toutes les images et actions.
- Documentation seule : matrice vide.
- Préparation : sélection complète ; maintenance sans diff.

Avant la première publication, un changement d'action seul ne peut pas tirer une
image épinglée inexistante : effectuer une qualification bootstrap complète.

La matrice couvre composant × architecture, avec `fail-fast: false` et
`max-parallel: 2`. GitHub limite une matrice à 256 jobs ; le parallélisme effectif
dépend des runners disponibles. Les résultats sont des fichiers par composant
et architecture, jamais un output matriciel partagé.

## Ajouter une opération ou un outil

1. Créer `oci/<verbe-approuvé>-<nom>/action.yml` et son README.
2. Réutiliser l'image de l'outil si elle existe ; sinon ajouter un Containerfile
   multi-stage avec bases épinglées et étape finale scratch, et `.containerignore`.
3. Ajouter une opération explicite au wrapper, sans interprétation de commande.
4. Ajouter fixtures positives/négatives et tests de scripts, d'image et d'action.
5. Ajouter un step `uses:` explicite dans `main.yml` pour le test GitHub natif :
   GitHub n'accepte pas un chemin `uses:` calculé à partir de la matrice.
6. Qualifier les deux architectures avant publication.

Pour les futurs contrôles `check`, chaque opération possédera sa propre image et
son exécutable ciblé. Les formats BitBake, Yocto, C, patches et templates LXC ne
sont pas annoncés comme lintés par le socle OCI.
