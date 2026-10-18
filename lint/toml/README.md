# Test TOML

Action Docker native avec son validateur Go spécialisé, utilisant
`github.com/pelletier/go-toml/v2` 2.4.3. [Interface](action.yml).

```yaml
- uses: tdesaules/github-actions/lint/toml@main
  with:
    files: '["cliff.toml", "lychee.toml"]'
```

Vérifie la syntaxe TOML et les clés dupliquées. Ce contrôle ne valide pas les
schémas applicatifs ni le style. Les fichiers vides sont acceptés. Les sources
ne sont pas modifiées. Un fichier absent, illisible ou invalide fait échouer
l'action. Rapport JSON sur stdout ; outputs `checked` et `passed`.

Image en qualification, pas encore publiée. Voir [le lot check](../../docs/checks.md).
