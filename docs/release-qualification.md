# Qualification complémentaire des releases

## Preuves obtenues

- Release stable `0.1.0` : push atomique commit/tag via App, validation des
  signatures et création de la GitHub Release réussis.
- [Sans changement, run 39119082070](https://github.com/tdesaules/github-actions/actions/runs/39119082070) :
  contrôles et lint réussis ; jobs build et release ignorés. Aucune nouvelle
  release n'est créée au même état Git.
- `tests/unit/test_release_recovery.py` utilise de vrais dépôts Git locaux : un
  hook refuse le push atomique, aucune ref distante partielle ne subsiste, puis
  un checkout neuf reprend avec les mêmes métadonnées de publication. Une autre
  campagne pousse un commit concurrent entre le précontrôle et le push final :
  le concurrent est préservé et le tag du perdant n'est pas publié.
- `scripts/test-registry.py --index-only` publie les deux enfants sur un vrai
  registre TLS, provoque un échec de confiance TLS avant l'index, puis reprend
  avec les mêmes archives. Une nouvelle tentative après succès conserve le
  même index et les mêmes enfants. Aucun tag d'image n'est créé.

Les campagnes locales isolées ne sont pas présentées comme des interruptions
réelles de GHCR ni comme une révocation du token App en cours de publication.
La vérification hébergée de réutilisation après modification ciblée reste à
effectuer une fois le premier lot `check` publié.

## Reprise opérationnelle

Après un échec, relancer les jobs échoués du même run préserve le commit de base
et les archives. Si `main` a avancé, la préparation refuse le push et conserve
les artefacts ; il faut une nouvelle préparation sur le nouveau commit. Un tag
existant n'est jamais déplacé. Après un push réussi mais une interruption du
workflow de tag, relancer ce workflow vérifie les références et crée la release
de façon idempotente.
