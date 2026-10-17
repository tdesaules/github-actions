# Validation du catalogue

## État actuel

Le [bootstrap 39034741897](https://github.com/tdesaules/github-actions/actions/runs/39034741897)
du commit `644bb15` est **entièrement réussi** : les dix builds/tests/scans
natifs, puis la publication des archives testées, sans reconstruction.
Les cinq index et leurs dix manifestes sont publiés sous `0.0.0-bootstrap`,
signés via GitHub OIDC et vérifiés avec l'identité exacte de `main.yml@refs/heads/main`
et l'émetteur `https://token.actions.githubusercontent.com`.
Les dix métadonnées d'action intègrent les digests issus du journal de publication.
Les rapports, checksums, signatures et le patch sont conservés dans les artefacts
de ce run. Le push automatique des releases SemVer reste conditionné à la
configuration de l'App (`RELEASE_APP_ID`, `RELEASE_APP_PRIVATE_KEY`).

Le [run du tag 39036476600](https://github.com/tdesaules/github-actions/actions/runs/39036476600)
est réussi : appartenance à l'historique de `main`, références immuables,
architectures et signatures sont vérifiées avant la création de la
[GitHub préversion 0.0.0-bootstrap](https://github.com/tdesaules/github-actions/releases/tag/0.0.0-bootstrap).
Le tag bootstrap a été poussé manuellement ; la création de cette GitHub Release
a bien été effectuée par le workflow. Une vérification locale indépendante des
cinq index et quinze signatures a également réussi sans authentification GHCR.

Le [run consommateur 39036429441](https://github.com/tdesaules/github-actions/actions/runs/39036429441)
est réussi pour les dix variantes, toutes avec `rebuild: false`. Il tire les
images GHCR épinglées puis réexécute les tests d'actions sur les deux architectures,
sans reconstruire les images du catalogue. Cela valide le parcours des changements
limités aux métadonnées d'action après la première publication.

## Historique des itérations

Le [run 39033229840](https://github.com/tdesaules/github-actions/actions/runs/39033229840)
du commit `5cb81d5` est **réussi** : `check`, `lint` et les dix variantes des
cinq images sur AMD64/ARM64 passent. Cela comprend les scans au seuil `high`,
le lanceur public Buildah, les actions natives, la publication TLS, les index
sans modification des digests et les signatures par clé éphémère. Les tests
Syft couvrent aussi la racine en lecture seule et les erreurs de permissions.
La tentative de bootstrap `39034584808` a échoué avant le démarrage des étapes
de publication : les groupes de concurrence du workflow et du job étaient
identiques. Ils sont désormais distincts. Une nouvelle qualification complète
est nécessaire au commit corrigé avant publication.

Le run `39030971131` confirme que les tests Buildah passent sur les deux
architectures ; ses jobs Buildah échouent ensuite au scan. La reconstruction
locale des cinq outils avec Go 1.27.2 et les dépendances corrigées passe le
seuil Grype `high`, avec la revue ciblée décrite ci-dessous. Les tests du registre
TLS et des signatures avec une clé éphémère passent également. La nouvelle
qualification GitHub complète est en cours. Les résultats des sections suivantes
décrivent les versions antérieures. Aucun résultat local ne vaut publication.

Voir la [méthode de correction et de revue](dependency-remediation.md).

Scans locaux des candidats reconstruits (avant les derniers ajouts de tests) :

| Image | Critical | High | Medium | Unknown | Revue ciblée |
|---|---:|---:|---:|---:|---:|
| Buildah | 0 | 0 | 4 | 1 | 1 |
| Skopeo | 0 | 0 | 2 | 1 | 1 |
| Syft | 0 | 0 | 0 | 0 | 0 |
| Grype | 0 | 0 | 1 | 0 | 1 |
| Cosign | 0 | 0 | 0 | 2 | 0 |

La colonne de revue correspond uniquement à `GO-2026-4887`, avec les preuves
de symboles conservées. Les correspondances Medium/Unknown restent visibles.

## Complément : chemin Buildah validé sur GitHub

Les campagnes `39030091316` et `39030331639` ont qualifié les runners hébergés
AMD64 et ARM64. Notre image Buildah scratch fonctionne pour la construction et
l'export OCI avec `--security-opt=seccomp=unconfined` et
`--security-opt=apparmor=unconfined`, sans `--privileged` ni capability ajoutée.
Un build négatif propage le code `42`. L'action native standard échoue toujours.
Voir le [comparatif et la décision recommandée](hosted-runner-qualification.md).
Les limites de la première qualification et les résultats locaux ci-dessous
sont conservés comme historique ; aucune publication n'a été effectuée.

## Première qualification GitHub

Le commit `3209df8` a été poussé sur `main` et exécuté dans le
[run 39029565392](https://github.com/tdesaules/github-actions/actions/runs/39029565392).
Les jobs `check` et `lint` passent. Les dix variantes de build échouent au
bootstrap Buildah sur les runners hébergés AMD64 et ARM64. Le journal AMD64
confirme `Error during unshare(CLONE_NEWUSER): Operation not permitted`.
Les actions candidates, scans et publication ne sont donc pas atteints.
Les erreurs d'upload d'artefacts sont secondaires : aucune archive n'a été créée.

Le profil Docker hébergé testé ne permet pas ce bootstrap. Il faut qualifier
un profil de runner autorisant les namespaces nécessaires avant de pouvoir
publier. Les résultats locaux ci-dessous ne constituent pas cette qualification.

Date : 17 octobre 2026. Hôte Linux AMD64, Podman 5.8.7 rootless.
La commande locale `docker` est un alias d'émulation Podman : aucun daemon
Docker réel ni runner GitHub n'a été utilisé pour ces résultats.

## Résultats confirmés

- Construction des cinq images finales `scratch` et smoke tests des outils.
- Bootstrap complet avec Buildah **conteneurisé**, y compris les étapes `RUN`
  de compilation ; aucun `--privileged`, aucun socket moteur monté.
- Versions finales : Buildah 1.43.4, Skopeo 1.22.3, Syft 1.54.1,
  Grype 0.120.1, Cosign 3.1.3 ; wrapper compilé avec Go 1.27.2.
- 14 tests unitaires Python et 11 tests Go réussis, `go vet`, syntaxe Python.
- Métadonnées des dix actions parsées/validées, workflow validé par actionlint
  1.7.7, cinq Containerfiles validés par Hadolint 2.14.0.
- Tests d'intégration des cinq outils : chemins contenant des espaces,
  construction, export/import OCI, SBOM de rootfs et d'archive, scan d'un SBOM,
  erreurs propagées et fixture source inchangée.
- Publication réelle dans un registre **local TLS temporaire**, lecture des
  digests distants et assemblage d'un index de deux fixtures AMD64/ARM64 sans
  changer les digests des enfants. Ce test ne constitue pas un build ARM64.
- Signature Cosign réelle de la fixture par digest et vérification réussie avec
  une clé éphémère sur ce registre local, sans publication dans un journal public.
- Vérification de la disponibilité AMD64 et ARM64 des six bases amont épinglées.

Commandes principales :

```text
task test-repository
task test-action-metadata
task initialize-bootstrap
task test-integration
task test-registry
python3 scripts/reports.py <image>
```

## Points qui empêchent encore de déclarer le catalogue publiable

- La qualification des véritables actions Docker sur GitHub et les builds/tests
  natifs ARM64 n'ont pas été exécutés. La matrice et les steps sont implémentés.
- Aucun package GHCR, tag Git ou GitHub Release n'a été publié.
- Le passage de l'identité GitHub OIDC et les signatures keyless ne sont pas
  validés localement ; ils nécessitent l'exécution du workflow autorisé.
- Les scans réels signalent des vulnérabilités dans les dépendances amont. Le
  seuil `high` est conservé et bloque la publication ; les rapports sont dans
  `.build/<image>/amd64/`. Une actualisation du wrapper Go a déjà supprimé la
  dépendance initiale à Go 1.25.3, mais ne corrige pas les binaires amont copiés.

Signalements des scans après actualisation des versions (ce sont des correspondances
de la base Grype, pas une démonstration d'exploitabilité ; des doublons ou faux
positifs peuvent nécessiter une analyse) :

| Image | Critical | High |
|---|---:|---:|
| Buildah | 2 | 22 |
| Skopeo | 8 | 26 |
| Syft | 3 | 13 |
| Grype | 3 | 16 |
| Cosign | 3 | 30 |

Le bootstrap permet de travailler localement. Il n'est pas une release prête
à consommer. Les références par digest seront produites à la publication, après
qualification des deux architectures et résolution/documentation des résultats
de scan selon la politique retenue.
