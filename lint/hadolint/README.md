# Test Containerfile

Action Docker native avec son exécutable Go et Hadolint 2.14.0 dans une image
`scratch` spécialisée. [Interface](action.yml).

```yaml
- uses: tdesaules/github-actions/lint/hadolint@main
  with:
    files: '["Containerfile", "build tools/Containerfile"]'
```

Les fichiers explicites sont contrôlés sans modification ; aucun glob n'est
développé. Les règles sont celles de Hadolint, avec sa configuration native et
ses commentaires d'exclusion. Un échec ou un fichier illisible produit un statut
non nul. Rapport JSON sur stdout ; outputs `checked` et `passed`.

Image en qualification, pas encore publiée. Voir [le lot check](../../docs/checks.md).
