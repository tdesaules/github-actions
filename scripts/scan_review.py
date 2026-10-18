"""Apply reviewed advisories only with symbol or exact patched-file evidence."""
import json
from pathlib import PurePosixPath


def patched_npm_review(sbom, policy):
    if not policy:
        return [], []
    artifacts = [a for a in sbom['artifacts'] if a['name'] == policy['package'] and a['type'] == 'npm']
    if not artifacts or not policy.get('files'):
        return [], []
    files = {}
    for item in sbom.get('files', []):
        files.setdefault(item['location']['path'], []).append(item)
    evidence = []
    for artifact in artifacts:
        if artifact['version'] != policy['version'] or not artifact.get('locations'):
            return [], []
        for location in artifact['locations']:
            if PurePosixPath(location['path']).name != 'package.json':
                return [], []
            root = PurePosixPath(location['path']).parent
            checked = {}
            for relative, expected in policy['files'].items():
                path = str(root / relative)
                matches = files.get(path, [])
                if len(matches) != 1:
                    return [], []
                digests = [d['value'] for d in matches[0].get('digests', []) if d['algorithm'] == 'sha256']
                if digests != [expected]:
                    return [], []
                checked[path] = expected
            evidence.append({'artifact_id': artifact['id'], 'version': artifact['version'],
                             'patched_files': checked, 'reference': policy['reference']})
    return [{'vulnerability': policy['vulnerability'], 'namespace': 'github:language:javascript',
             'package': {'name': policy['package'], 'version': policy['version'], 'type': 'npm'},
             'reason': policy['reason']}], evidence


def review(sbom, policy):
    artifacts = [a for a in sbom['artifacts'] if a['name'] == policy['package']]
    evidence = []
    rules = []
    for artifact in artifacts:
        symbols = artifact.get('metadata', {}).get('symbols', {})
        # Missing symbol capture must never be interpreted as absence of code.
        if not symbols or not any(symbols.values()):
            return {'ignore': []}, []
        affected = policy['affected_package']
        if any(p == affected or p.startswith(affected+'/') for p in symbols):
            return {'ignore': []}, []
        evidence.append({'artifact_id': artifact['id'], 'version': artifact['version'],
                         'linked_packages': sorted(symbols), 'reference': policy['reference']})
        rules.append({'vulnerability': policy['vulnerability'],
                      'namespace': 'govulndb:language:go',
                      'package': {'name': artifact['name'], 'version': artifact['version'], 'type': 'go-module'},
                      'reason': policy['reason']})
    patched_rules, patched_evidence = patched_npm_review(sbom, policy.get('patched_npm'))
    return {'ignore': rules + patched_rules}, evidence + patched_evidence
