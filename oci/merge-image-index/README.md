# merge-image-index

Action Docker native utilisant son image spécialisée `oci-merge-image-index`.
[Interface complète](action.yml).

`sources` est un tableau JSON de deux chemins d'archives testées, AMD64 et ARM64.
`destination` est `docker://registre/dépôt`, sans tag ni digest. L'exécutable Go
assemble un index déterministe puis Skopeo publie les enfants et l'index avec
`--all --preserve-digests`, directement par digest, sans reconstruction ni tag.
Les sorties `image` et `digest` identifient l'index publié ; `output` conserve
son manifeste JSON exact (par défaut `image-index.json`).

L'orchestrateur de publication exige les deux résultats matriciels et vérifie
que les manifestes de l'index publié correspondent aux digests des archives.
Cette action ne remplace pas cette validation de publication.

Le test public négatif couvre une liste vide. Un registre TLS temporaire permet
de tester l'assemblage positif et la conservation des digests sur chaque runner.
