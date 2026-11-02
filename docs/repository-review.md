# Revue du catalogue après les premières PR Renovate

Révision source revue : `80bffbb` (merges Node et Go 1.27.3).
Les preuves ci-dessous distinguent les tests locaux, les images déjà publiées et
la qualification complète de cette nouvelle révision.

## Résultats vérifiés

- 47 opérations et images dédiées, avec action, Containerfile multistage terminé
  par `scratch`, exécutable Go et README. 46 actions Docker natives et l'exception
  Buildah composite documentée.
- Six jobs composés d'actions, sans step `run` dans le workflow principal. Les
  actions externes sont épinglées par commit et les images par digest.
- Les 47 index publiés référencés au moment de la revue sont accessibles sans
  authentification ; leur checksum correspond au digest et chacun expose Linux
  AMD64 et ARM64. Cette vérification concerne les pins existants, pas des candidats
  dont la publication est encore en cours.
- `go test ./...`, 28 tests Python et `python3 scripts/ci.py validate` réussis.
- Validation de 47 métadonnées d'actions, Actionlint, 47 Containerfiles, huit
  scripts shell, 64 documents Markdown, 50 fichiers YAML et deux TOML réussie.
  `tests/runner/action.yml` est une fixture de tests, pas une opération publiée.
- Revue des contrôles de publication : ensemble attendu, révision exacte,
  qualification, checksums, version candidate uniforme, signatures des plateformes
  et de l'index, refus d'un main avancé et push atomique sans déplacement de tag.
- Scans sans reconstruction et reprise entre tentatives démontrés sur GitHub,
  jusqu'à la release `1.3.1` : voir les
  [preuves détaillées](qualification-corrections.md).

Les rapports locaux sont conservés sous `/tmp/opencode/final-review-structure.json`,
`/tmp/opencode/final-review-ghcr.json` et `/tmp/opencode/final-review-lints.log`.
Ces chemins sont des preuves de travail locales, pas des assets de release.

## Corrections implémentées après la revue

Les corrections ci-dessous ont passé les tests locaux et la qualification native
du run `41396008309`. Leurs images sont publiées et épinglées dans `1.4.0`.
Les validations OIDC et du hook Renovate réel doivent être distinguées de ces
tests de construction, de runtime et de scans.

### Jeton OIDC dans les arguments du processus Cosign

Au moment de la revue, `shared/publication/signature.go` et `shared/runtime/main.go` transmettaient
le jeton obtenu par le wrapper à Cosign avec `--identity-token`. Les cinq inputs
publics de mot de passe de registre sont bien transmis par environnement, mais
le contrat « secrets hors argv » n'était donc pas respecté jusqu'au processus
enfant du signataire. Aucune fuite effective n'a été constatée pendant cette revue.

La correction utilise le support de fichier de `--identity-token` dans Cosign
3.1.3 : fichier `0600`, répertoire temporaire `0700` hors workspace, suppression
après succès ou échec. Les deux chemins de signature partagent ce mécanisme.
Les tests contrôlent le contenu reçu par le signataire, les permissions,
l'absence du jeton dans les arguments enregistrés et le nettoyage après échec.
La préparation de `1.4.0` utilisait les signataires précédemment épinglés.
La préparation `41420324041` de `1.4.1` a depuis exercé avec succès le signataire
corrigé en OIDC réel, puis vérifié ses signatures.

### Versions sources amont non extraites par le manager Renovate

`renovate.json:19` reconnaît `module@vVersion`. Les appels dans
`shared/runtime/compile-{skopeo,cosign,syft,grype}.sh:3` utilisent en revanche
`module vVersion h1:checksum` pour les sources amont. La reproduction locale
confirme que ces quatre versions ne sont pas extraites par cette expression.
Les dépendances correctives au format `module@version`, les bases, actions,
modules du dépôt et dépendances npm sont bien dans le périmètre configuré.

`renovate-tool-sources.json` complète maintenant l'extraction. Cette configuration
globale est embarquée avec le runner corrigé, afin de ne pas activer un hook absent
des anciennes images. L'extraction locale par Renovate détecte huit déclarations
sources, dont les quatre outils signalés, Buildah, Actionlint et Distribution.

Un hook fixe `/usr/local/bin/update-tool-checksums` recalcule les sommes après la
mise à jour des versions, avec `go mod download` et la vérification sum.golang.org
imposée. Seule cette commande est autorisée. Il valide l'identité du module, sa
version et le format h1, et résout toutes les sommes avant de modifier les recettes.
La configuration de dépôt conserve les gestionnaires existants. La vérification
de somme du build est conservée ; aucun checksum Git n'est substitué à la somme Go.

Tests locaux : configuration et extraction Renovate réelles, mise à jour d'une
somme h1 volontairement périmée dans une fixture conteneurisée, rejet de sommes ou
identités invalides et de l'indisponibilité du service. Le dry-run authentifié du
runner publié est validé par `41407460725`. L'écriture des checksums par le hook
hébergé est désormais validée par les PR #6/#7, décrites ci-dessous.

L'exécution réelle `41408917330` a créé les PR #4/#5 pour des dépendances au format
`module@version`, sans exercer le hook des sources. La tentative `41409848683`
s'est arrêtée avec `repository-changed` malgré un statut Actions vert. Une branche
Actionlint avait été créée avec la version seule et un checksum périmé. Cet état
invalide a depuis été remplacé par la régénération correcte de la PR #6.

Le diagnostic en dry-run authentifié confirme l'exécution du hook et deux entrées
pour le même chemin dans la préparation du commit (manager puis artefact corrigé).
Le dépôt désactive désormais `platformCommit` pour utiliser les commits Git du
runner. Ce changement seul n'avait pas résolu le problème d'environnement décrit
ci-dessous ; les PR finales valident l'ensemble corrigé.

Ce diagnostic a aussi reproduit `spawn npm ENOENT` pour les lockfiles npm. Le
runtime npm 12.2.1 est ajouté depuis un lockfile dédié, avec Undici 6.29.1 remplaçant
sa copie embarquée vulnérable. Le candidat reconstruit produit un lockfile avec
ses intégrités et passe le scan Grype complet au seuil `high`, sans exclusion
supplémentaire. L'ancien npm fourni par la base Node et npm 12.2.1 non corrigé
avaient été rejetés par ce même scan avant la correction.

La release `1.4.1` a publié ce runtime npm. Sa préparation `41420324041` a aussi
exercé avec succès le nouveau signataire OIDC par fichier. Les runs du tag
`41421033026` et de main `41421032426` ont réussi.

La maintenance `41421256070` est verte au niveau Actions, mais Renovate a encore
terminé avec `repository-changed`. Le passage aux commits Git n'a donc pas suffi.
Un diagnostic instrumenté confirme que le helper calcule la bonne somme lorsque
son environnement est inscriptible. En revanche, l'environnement filtré du hook
retire GOPATH/GOCACHE : avec un HOME non inscriptible, l'ancien helper échoue.

La correction crée des caches Go temporaires privés, indépendants de HOME, et
exécute les téléchargements hors checkout. Le même test conteneurisé avec HOME
non inscriptible passe après correction ; les tests vérifient aussi le nettoyage
des caches sur réponse valide ou invalide. Les erreurs d'écriture des statuts
GitHub et les résultats des hooks sont remontés explicitement dans les logs
Renovate pour éviter qu'un nouvel arrêt générique ne masque sa cause.

### Validation finale hébergée

Le run `41443035104` a qualifié le runner corrigé en AMD64/ARM64. La préparation
`41444214574` a réutilisé ses archives exactes et publié `1.4.2`. Les runs main
`41445016189` et tag `41445016266` sont verts ; la GitHub Release est publiée.

La maintenance `41445096638` a réussi les SBOMs, scans, liens et l'exécution réelle
de Renovate. Ses logs montrent le hook exécuté pour Actionlint et Cosign, les PR
numéros 6 et 7 créées et `result: done`, `exitCode: 0`. Les diffs mettent à jour ensemble
la version et la somme h1, confirmée indépendamment par `go mod download` :

- Actionlint `v1.7.12` : `h1:vQ4GeJN86C0QH+gTUQcs8McmK62OLT3kmakPMtEWYnY=`.
- Cosign `v3.1.4` : `h1:d3UHS8oJebMf6XUhPtWv9weU1ZYRyXq9HJ7QPccmqvY=`.

Les deux écarts de cette revue sont donc corrigés et exercés en conditions réelles.
Cela valide le fonctionnement du bot, pas la compatibilité de chaque mise à jour
proposée : les PR de dépendances conservent leur propre qualification avant fusion.

## Documentation rectifiée

La documentation d'architecture désignait encore le sélecteur Python comme
implémentation active et annonçait l'absence de shell/compilateur dans toutes les
images. Elle décrit maintenant les actions Go réellement appelées et les runtimes
nécessaires aux tests et à Renovate. Les documents de qualification ne présentent
plus les validations acquises en `1.3.1` comme des travaux futurs.

## Qualification après merges

Le run Node `41352321746` a réussi sur les deux architectures. Il n'a qualifié
que l'image Renovate ; le run du merge Go précédent avait été annulé par ce push.
La préparation complète `41352590229` sur `80bffbb` a passé les 94 builds et tests,
mais refusé le push final parce que main avait avancé. La campagne `41396008309`
sur `3c892b9` a ensuite qualifié et publié le lot complet jusqu'au tag `1.4.0`.
Le tag `41407237512`, main `41407237711` et la maintenance `41407460725` sont
réussis ; la GitHub Release `1.4.0` est publiée. Les détails sont conservés dans
les [preuves de qualification](qualification-corrections.md).
