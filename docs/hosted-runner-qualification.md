# Qualification Docker sur runners GitHub hébergés

Le mode manuel `qualify-runner` de `main.yml` teste Ubuntu 24.04 AMD64 et ARM64
natifs. Il ne publie aucune image. Il utilise Buildah amont épinglé et les mêmes
options internes que le socle : stockage `vfs`, isolation `chroot`, réseau du
conteneur extérieur.

## Protocole

Chaque profil lance un conteneur neuf et construit une image avec un vrai `RUN`
qui écrit `/proof`, puis exporte une archive OCI. Un échec ou un timeout est
enregistré dans `profiles.json` avec le journal complet du profil. Les profils
font varier `SYS_ADMIN`, seccomp et AppArmor, séparément et en combinaison.
`--privileged` sert de témoin, pas de configuration recommandée par défaut.

Un step `uses: ./tests/runner` exécute ensuite le même script dans une véritable
action Docker native. Son résultat est conservé dans `native-outcome.txt`.
Le succès du job de diagnostic signifie que les preuves ont été collectées,
pas que tous les profils sont compatibles. Lire les résultats des artefacts.

## Critères de décision

1. Privilégier un profil fonctionnant avec les paramètres du runner GitHub natif.
2. Si des options Docker supplémentaires sont indispensables, distinguer une
   commande `docker run` explicitement configurée d'une action Docker native :
   `runs` ne permet pas de transmettre ces options.
3. Éviter de modifier globalement le daemon Docker du runner pour tous les steps.
4. Qualifier le build complet de notre image scratch avant de généraliser un
   résultat obtenu avec la fixture minimale.
5. Maintenir la séparation entre qualification runtime, scans de vulnérabilités
   et autorisation de publication. Un build réussi ne valide pas les deux autres.

Le premier run standard `39029565392` échouait au bootstrap, avant les actions
candidates. Ce protocole permet de tester explicitement cette distinction.

Référence : [métadonnées des actions Docker natives](https://docs.github.com/en/actions/reference/workflows-and-actions/metadata-syntax#runs-for-docker-container-actions).
Les champs pris en charge comprennent l'image, l'entrypoint, les arguments et
l'environnement ; ils n'exposent pas les capabilities ou les profils de sécurité
du conteneur extérieur.
