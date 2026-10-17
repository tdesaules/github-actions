# merge-image-index

Action composite lançant l'image Buildah avec le profil Docker qualifié.
[Interface complète](action.yml).

`sources` est un tableau JSON de deux chemins d'archives testées, AMD64 et ARM64.
`destination` est une référence `docker://`. L'action crée et publie un index
avec `buildah manifest`, sans reconstruire ses images. Publier préalablement les
deux archives dans le même dépôt avec `publish-image`. Seul l'index est poussé :
recopier les enfants via Buildah risquerait de recompresser leurs couches et de
modifier leurs digests. Le registre doit déjà contenir les manifestes référencés.

L'orchestrateur de publication exige les deux résultats matriciels et vérifie
que les manifestes de l'index publié correspondent aux digests des archives.
Cette action ne remplace pas cette validation de publication.

Le test public négatif couvre une liste vide. Un registre TLS temporaire permet
de tester l'assemblage positif et la conservation des digests sur chaque runner.
