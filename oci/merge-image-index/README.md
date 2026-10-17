# merge-image-index

Action Docker utilisant l'image Buildah. [Interface complète](action.yml).

`sources` est un tableau JSON de deux chemins d'archives testées, AMD64 et ARM64.
`destination` est une référence `docker://`. L'action crée et publie un index
avec `buildah manifest`, sans reconstruire ses images.

L'orchestrateur de publication exige les deux résultats matriciels et vérifie
que les manifestes de l'index publié correspondent aux digests des archives.
Cette action ne remplace pas cette validation de publication.

Le test natif négatif couvre une liste vide ; la création effective d'un index
est exercée par la publication bootstrap, à exécuter avec les deux architectures.
