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

## Résultats du premier comparatif

[Run 39030091316](https://github.com/tdesaules/github-actions/actions/runs/39030091316),
commit `6a0f32d`, 17 octobre 2026. Même résultat sur les deux architectures :

| Profil extérieur | AMD64 | ARM64 | Cause d'échec observée |
|---|---|---|---|
| Standard | Échec | Échec | `unshare(CLONE_NEWUSER)` refusé |
| seccomp désactivé | Échec | Échec | remount `/` refusé |
| AppArmor désactivé | Échec | Échec | `unshare(CLONE_NEWUSER)` refusé |
| `SYS_ADMIN` ajouté | Échec | Échec | remount `/` refusé |
| seccomp + AppArmor désactivés | Succès | Succès | — |
| `SYS_ADMIN` + seccomp désactivé | Échec | Échec | remount `/` refusé |
| `SYS_ADMIN` + AppArmor désactivé | Échec | Échec | hausse `RLIMIT_NOFILE` refusée |
| `SYS_ADMIN` + seccomp + AppArmor désactivés | Échec | Échec | hausse `RLIMIT_NOFILE` refusée |
| `--privileged` | Succès | Succès | — |
| Véritable action Docker native standard | Échec | Échec | profil standard du runner |

Les erreurs de limite de fichiers n'établissent pas une impossibilité du profil
avec `SYS_ADMIN` : elles indiquent un autre réglage nécessaire. Ce profil n'est
pas retenu puisqu'un profil sans capability supplémentaire réussit déjà.

## Chemin retenu pour la qualification complète

Tester notre image Buildah scratch avec les deux options **limitées au conteneur
Buildah** :

```text
--security-opt=seccomp=unconfined --security-opt=apparmor=unconfined
```

Cela retire deux protections du conteneur : ce n'est pas équivalent au profil
standard, même sans `--privileged`. Le daemon et les autres conteneurs ne sont
pas reconfigurés. Le profil devra être appliqué aux opérations qui utilisent
Buildah, y compris l'assemblage d'index, puis validé pour chacune.

Si le test scratch confirme le résultat, le chemin opérationnel recommandé est
une exception explicite au lancement natif pour Buildah : un step Docker
configuré, utilisant toujours notre image préconstruite épinglée. Skopeo, Syft,
Grype et Cosign peuvent conserver leur contrat d'action native, sous réserve de
leurs propres tests GitHub. Aucun outil métier n'est installé sur le runner.

Cette exception modifie l'exigence initiale « toutes les actions Docker natives ».
Elle n'est pas appliquée silencieusement aux actions publiques. Les autres
options sont un builder distant ou des runners configurés ; aucune de ces
infrastructures n'est actuellement disponible dans ce projet.

## Qualification de notre image scratch : réussie

[Run 39030331639](https://github.com/tdesaules/github-actions/actions/runs/39030331639),
commit `3caa51f`, sur les runners hébergés AMD64 et ARM64 natifs :

- Bootstrap de notre image Buildah scratch avec compilation réelle : succès.
- Chargement de l'archive candidate et smoke test : succès.
- Construction avec cette candidate d'une image contenant `RUN` : succès.
- Export et vérification de l'architecture de l'archive OCI : succès.
- `RUN exit 42` : code `42` propagé sur les deux architectures, aucune archive
  produite par le build en échec.
- Action Docker native standard : échec sur les deux architectures.

Digests des manifestes de **fixtures** exportées, pas d'images publiées :

| Architecture | Digest |
|---|---|
| AMD64 | `sha256:f1722146b35cf4205dc7e834955d7aa3999519f4309eb9cb3c70b0fdb44026a7` |
| ARM64 | `sha256:4700f9e03bd1249ef7ebbc188a7c9752597e35f7c3f91a5d35f5b92724edccc9` |

Les artefacts `qualification-ubuntu-24.04` et `qualification-ubuntu-24.04-arm`
contiennent les résultats structurés et les journaux de profils. Le journal du
job contient les builds complets. Les deux campagnes ont exécuté au total
36 essais de profils et 4 appels natifs, auxquels s'ajoutent les builds scratch
positifs/négatifs de la deuxième campagne.

**Décision technique recommandée :** conserver les runners hébergés et Buildah
conteneurisé ; autoriser une exception explicite de lancement Docker pour les
opérations Buildah. Ne pas généraliser `--privileged`, ne pas reconfigurer le
daemon du runner. L'interface publique n'a pas été convertie : cette évolution
nécessite d'accepter l'exception à l'exigence Docker native.

Cette campagne qualifie la construction Buildah, pas le catalogue entier :
l'assemblage d'index avec ce profil, les autres actions natives, les scans et
la publication OIDC/GHCR restent à qualifier dans le pipeline complet.
