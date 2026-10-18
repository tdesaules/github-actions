# Test shell

Action Docker native avec son exécutable Go et ShellCheck 0.11.0 dans une image
`scratch` spécialisée. [Interface](action.yml).

```yaml
- uses: tdesaules/github-actions/check/test-shell@main
  with:
    files: '["scripts/build", "scripts/setup.script", "scripts/test.sh"]'
```

Les fichiers sont sélectionnés explicitement, y compris sans extension ou avec
une extension `.script`. Leur shebang détermine le dialecte ShellCheck. Les
scripts ne sont jamais exécutés ; aucun glob ni fragment shell n'est interprété.
Rapport JSON sur stdout ; outputs `checked` et `passed`. Toute erreur de lecture
ou diagnostic bloquant de ShellCheck fait échouer l'action.

Image en qualification, pas encore publiée. Voir [le lot check](../../docs/checks.md).
