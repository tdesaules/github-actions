# import-image

Action Docker Skopeo. [Interface complète](action.yml).

Importe une archive `oci-archive:chemin.tar` vers un layout `oci:répertoire:nom`.
Le layout persiste dans le workspace. Ce n'est pas un `docker load` dans le
stockage du runner : les actions restent indépendantes d'un socket moteur.

La fixture valide est produite par `scripts/testing.py` sans dépendre de Buildah.
Le test invalide vérifie l'échec d'une archive absente. L'aller-retour couvre
les chemins contenant des espaces et ne modifie pas la fixture source.
