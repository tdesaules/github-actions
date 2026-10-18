# Test GitHub workflow

Image spécialisée `check-test-github-workflow`, action Docker native, entrypoint
Go et actionlint 1.7.7 reconstruits avec le compilateur Go épinglé.
[Interface](action.yml).

```yaml
- uses: tdesaules/github-actions/lint/actionlint@main
  with:
    files: '[".github/workflows/main.yml"]'
```

`files` est un tableau JSON non vide de fichiers explicites, sans expansion de
globs. Les chemins avec espaces sont conservés. Les fichiers sont uniquement
lus ; chaque résultat est écrit dans un rapport JSON sur stdout. Les outputs
`checked` et `passed` sont écrits même si un contrôle échoue. L'action échoue si
un fichier est invalide, absent ou illisible. Les intégrations ShellCheck et
Pyflakes d'actionlint sont désactivées ; ShellCheck a sa propre action.

Les nouvelles images sont en qualification : attendre leur épinglage par digest
et leur release avant consommation. Voir [le lot check](../../docs/checks.md).
