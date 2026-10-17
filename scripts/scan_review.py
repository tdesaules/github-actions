"""Apply one reviewed advisory only when binary symbol evidence proves absence."""
import json


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
    return {'ignore': rules}, evidence
