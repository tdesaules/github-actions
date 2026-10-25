# new-sbom

Action Docker et image propriétaire Syft. [Interface complète](action.yml).

Sources explicites : `oci-archive:`, `oci-dir:`, `registry:` ou `dir:` pour un
rootfs. Formats : `spdx-json` (défaut), `cyclonedx-json`, `syft-json`. `output` désigne un
fichier du workspace et `sbom` expose son chemin.

Un scan de rootfs n'exécute pas son contenu. Les tests génèrent réellement un
SBOM à partir d'un répertoire et d'une archive, y compris avec espaces dans les
chemins, et vérifient l'échec d'une source inexistante.
