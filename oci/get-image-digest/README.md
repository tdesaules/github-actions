# get-image-digest

Action Docker Skopeo. [Interface complète](action.yml).

Calcule SHA-256 sur les octets de `skopeo inspect --raw`. Pour une image
multiarchitecture, il s'agit du digest de l'index et non du manifeste de la
plateforme du runner. `expected-digest` permet d'imposer une égalité exacte.

Outputs : `digest` et `reference` qualifiée par digest. L'accès à un registre
privé utilise les mêmes inputs d'authentification que `publish-image`.

Le workflow teste cette opération contre un index amont épinglé connu.
