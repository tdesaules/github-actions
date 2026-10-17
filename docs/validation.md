# Validation effectuée localement

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
