# Changelog

## 0.0.0-bootstrap

- Dix opérations OCI, avec cinq images finales `scratch` publiées sur GHCR.
- Construction et tests natifs AMD64/ARM64 sur les runners GitHub hébergés.
- Références immuables, signatures GitHub OIDC des index et des manifestes.
- Publication des archives testées sans reconstruction ni changement de digest.
- Lanceur Docker qualifié pour Buildah ; huit autres actions Docker natives.
- Contrôles de vulnérabilités au seuil `high`, SBOMs et preuves de revue conservés.
- Planification SemVer, réutilisation des images inchangées et push atomique
  commit/tag implémentés ; configuration GitHub App requise pour l'automatisation.

Validation : https://github.com/tdesaules/github-actions/actions/runs/39034741897
