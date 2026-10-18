# Test YAML

Action Docker native avec son validateur Go spécialisé, utilisant
`go.yaml.in/yaml/v3` 3.0.5. [Interface](action.yml).

```yaml
- uses: tdesaules/github-actions/check/test-yaml@main
  with:
    files: '["taskfile.yml", "configuration with spaces.yaml"]'
```

Vérifie la syntaxe et les clés de mapping dupliquées, dans tous les documents
d'un flux YAML. Ce contrôle ne valide pas les schémas applicatifs ni un style
de présentation. Les fichiers vides sont acceptés comme flux sans document.
Les sources ne sont pas modifiées. Un fichier absent, illisible ou invalide fait
échouer l'action. Rapport JSON sur stdout ; outputs `checked` et `passed`.

Image en qualification, pas encore publiée. Voir [le lot check](../../docs/checks.md).
