# Migration des consommateurs Pantavisor

Aucun fichier de `pantavisor-system`, `pantavisor-core` ou `pantavisor-k0s` n'est
modifié par cette livraison.

Après publication et qualification du catalogue :

1. Vérifier un runner Linux Docker compatible avec Buildah imbriqué pour chaque
   architecture ; ne pas annoncer ARM64 à partir d'un test AMD64.
2. Conserver les jobs du consommateur et remplacer uniquement leurs steps outils
   par `uses: tdesaules/github-actions/oci/<opération>@<version-sans-v>`.
3. Passer explicitement `Containerfile` ou `Containerfile.tools`, le contexte,
   la plateforme, les labels et les chemins de sorties.
4. Utiliser les archives OCI comme contrat entre jobs ; ne pas compter sur la
   persistance du stockage interne des actions Docker.
5. Générer le SBOM de l'archive, scanner ce SBOM, archiver les rapports puis
   publier cette même archive. Signer et vérifier par digest.
6. Fournir `packages: write` uniquement au job de publication et `id-token: write`
   uniquement au job de signature keyless.

Les scripts Shell sans extension, `.script`, recettes BitBake, configurations
Yocto, C/headers, patches et `.conf.in` seront traités dans le futur lot lint,
avec classification par contenu/chemin. Le présent socle ne revendique aucun
contrôle de ces formats.
