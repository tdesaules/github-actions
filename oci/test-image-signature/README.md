# test-image-signature

Action Docker Cosign. [Interface complète](action.yml).

Exige une référence par digest. En keyless, `identity` et `issuer` sont
obligatoires et transmis comme contraintes exactes à Cosign ; aucun wildcard
d'identité ni désactivation implicite de la vérification du journal.

Une clé publique peut être fournie avec `key` pour une signature à clé explicite.
`output` contient le résultat JSON de la vérification réussie. Un échec conserve
un code de sortie non nul.

`transparency-log: 'false'` n'est accepté qu'avec une clé publique explicite pour
les registres privés/tests. La signature cryptographique reste vérifiée.

Le workflow de publication vérifie l'identité du workflow `main.yml` sur `main`
et l'émetteur `https://token.actions.githubusercontent.com`.
