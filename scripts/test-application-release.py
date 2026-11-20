#!/usr/bin/env python3
"""Exercise the local fixed-operation binaries against disposable Git and OCI fixtures."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[1]
def test_application_release_contracts():
 with tempfile.TemporaryDirectory() as tmp:
    root = Path(tmp)
    env = dict(os.environ, GIT_CONFIG_GLOBAL=os.devnull, GITHUB_REPOSITORY='fixture/lab', GITHUB_WORKFLOW='main', GITHUB_OUTPUT=str(root/'output'), GITHUB_EVENT_PATH=str(root/'event.json'),
               GITHUB_EVENT_NAME='workflow_dispatch', GITHUB_REF='refs/heads/main')

    def run(*cmd, ok=True):
        if cmd[0]=='./check':
            paths=[]
            if env['GITHUB_EVENT_NAME']=='push' and env['GITHUB_REF']=='refs/heads/main':
                paths=run('git','diff','--name-only',json.loads((root/'event.json').read_text())['before'],'HEAD').splitlines()
            event='maintenance' if env['GITHUB_WORKFLOW']=='maintenance' else env['GITHUB_EVENT_NAME']
            cmd=(*cmd,'--files='+json.dumps(paths),'--event='+event,'--ref='+env['GITHUB_REF'])
        if cmd[0]=='./tag-images':env['APP_DIGESTS']=json.dumps({'bastion':env['BASTION_DIGEST'],'provision':env['PROVISION_DIGEST']})
        result = subprocess.run(cmd, cwd=root, env=env, capture_output=True, text=True)
        assert (result.returncode == 0) == ok, (cmd, result.stderr)
        return result.stdout.strip()

    for name, package in [('check','get-image-matrix'), ('verify','test-build-artifact'), ('tag-images','complete-release')]:
        subprocess.run(['go','build','-o',str(root/name),'./workflow/'+package], cwd=ROOT, check=True)
    env.update(APPLICATION_DEFINITION='infra/infra-definition.sample.json')
    run('git', 'init', '-qb', 'main'); run('git', 'config', 'user.name', 'Fixture'); run('git', 'config', 'user.email', 'fixture@example.org')
    (root/'oci/bastion').mkdir(parents=True); (root/'oci/bastion/Containerfile').touch(); (root/'infra').mkdir(); (root/'infra/infra-definition.sample.json').write_text(json.dumps({'images':{n:'ghcr.io/fixture/lab/'+n+'@sha256:'+'a'*64 for n in ('bastion','provision')}}));run('git','add','oci','infra')
    run('git', 'commit', '--allow-empty', '-qm', 'initial'); before = run('git', 'rev-parse', 'HEAD')
    (root/'oci/provision').mkdir(parents=True); (root/'oci/provision/Containerfile').touch()
    run('git', 'add', 'oci'); run('git', 'commit', '-qm', 'fix: change provision')
    (root/'event.json').write_text(json.dumps({'before':before})); env.update(GITHUB_EVENT_NAME='push', GITHUB_SHA=run('git','rev-parse','HEAD'))
    (root/'output').touch(); run('./check'); assert 'images=["provision"]' in (root/'output').read_text()
    def plan():
        (root/'output').write_text('')
        run('./check')
        return dict(line.split('=', 1) for line in (root/'output').read_text().splitlines())

    # A package-only publication has no release tag. Maintenance must still
    # report an immutable subject for every deployed image.
    env.update(GITHUB_EVENT_NAME='schedule', GITHUB_WORKFLOW='maintenance')
    assert set(json.loads(plan()['digests'])) == {'bastion', 'provision'}
    env.update(GITHUB_EVENT_NAME='push', GITHUB_WORKFLOW='main')
    def change(path):
        before = run('git', 'rev-parse', 'HEAD')
        file = root/path
        file.parent.mkdir(exist_ok=True, parents=True)
        file.touch()
        run('git', 'add', path)
        run('git', 'commit', '-qm', 'test: change fixture')
        (root/'event.json').write_text(json.dumps({'before': before}))
        env['GITHUB_SHA'] = run('git','rev-parse','HEAD')
        return plan()

    p = change('.github/scripts/smoke-provision.py')
    assert p['images'] == '["provision"]' and p['build_images'] == '[]' and p['scan_only'] == '[]'
    p = change('.grype.yaml')
    assert p['images'] == p['scan_only'] == '["bastion","provision"]' and p['has_build'] == 'false'
    p = change('docs/readme.md')
    assert p['has_build'] == p['has_tests'] == p['platform'] == 'false'
    env.update(GITHUB_EVENT_NAME='workflow_dispatch', GITHUB_REF='refs/heads/topic'); run('./check',ok=False)
    env['GITHUB_REF']='refs/heads/main'; rev=env['GITHUB_SHA']; dist=root/'dist'; dist.mkdir(); digest='sha256:'+'a'*64
    for name in ('bastion','provision'):
        env.update(COMPONENT=name,CANDIDATE_DIGEST=digest); archive=dist/'image.oci.tar'; archive.write_bytes(b'fixture')
        (dist/'build.json').write_text(json.dumps(dict(image=name,arch='amd64',version=rev,revision=rev,tested=False,archive_sha256=hashlib.sha256(archive.read_bytes()).hexdigest())))
        q=dist/name/'qualified.json';q.parent.mkdir();env.update(QUALIFICATION=str(q),TESTED_DIGEST=digest)
        args=['./verify','--archive='+str(archive),'--receipt='+str(dist/'build.json'),'--image='+name,'--architecture=amd64','--revision='+rev,'--version='+rev]
        archive.write_bytes(b'corrupt');run(*args,ok=False);archive.write_bytes(b'fixture');run(*args)
        env['TESTED_DIGEST']='';run(*args);archive.write_bytes(b'changed');run(*args,ok=False)
    run('git','init','--bare','-q','remote');run('git','remote','add','origin',str(root/'remote'));run('git','push','origin','main');env.update(VERSION='1.0.0',BASTION_DIGEST=digest,PROVISION_DIGEST='wrong');run('./tag-images',ok=False)
    env['PROVISION_DIGEST']=digest;run('./tag-images');env.update(GITHUB_REF='refs/tags/1.0.0',GITHUB_EVENT_NAME='push');run('./check');assert f'"bastion":"{digest}"' in (root/'output').read_text()
    env.update(GITHUB_REF='refs/heads/main', GITHUB_EVENT_NAME='workflow_dispatch', GITHUB_WORKFLOW='release')
    p = plan()
    assert p['build_images'] == '[]' and p['has_tests'] == 'true' and set(json.loads(p['reused'])) == {'bastion','provision'}

if __name__ == '__main__':
    test_application_release_contracts()
