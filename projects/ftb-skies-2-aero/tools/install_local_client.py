"""Install the verified client release with touched-file backup and rollback."""
from datetime import datetime
import hashlib
import json
from pathlib import Path
import subprocess
import zipfile
from runtime_config import instance_path

PROJECT=Path(__file__).resolve().parents[1]
INSTANCE=instance_path()
RELEASE=PROJECT/'output/releases/ftb-skies-2-aero-1.13.0-ko-v1'


def sha(value):return hashlib.sha256(value).hexdigest()


def main():
    state=json.loads((INSTANCE/'minecraftinstance.json').read_text(encoding='utf-8'))
    assert state['installedModpack']['installedFile']['fileName']=='ftb-skies-2-aero-1.13.0.zip'
    check=subprocess.run(['powershell','-NoProfile','-Command',
        "@(Get-CimInstance Win32_Process | Where-Object { $_.Name -match '^javaw?\\.exe$' }).Count"],
        check=True,capture_output=True,text=True)
    if int(check.stdout.strip())!=0:raise RuntimeError('Close Java/game processes before installation')
    package=RELEASE/'ftb-skies-2-aero-1.13.0-ko-v1-client.zip'
    receipt=json.loads((RELEASE/'release-receipt.json').read_text(encoding='utf-8'))
    asset=next(a for a in receipt['assets'] if a['name']==package.name)
    assert asset['digest']=='sha256:'+sha(package.read_bytes())
    with zipfile.ZipFile(package) as z:
        assert z.testzip() is None
        manifest=json.loads(z.read('translation-manifest.json'))
        files={n:z.read(n) for n in manifest['files']}
    assert len(files)==23 and {n:sha(v) for n,v in files.items()}==manifest['files']
    source=json.loads((PROJECT/'output/preparation-20261011-03/source-manifest.json').read_text(encoding='utf-8'))
    original={};targets={}
    for name,data in files.items():
        target=(INSTANCE/name).resolve()
        if not target.is_relative_to(INSTANCE):raise ValueError('Unsafe destination')
        targets[name]=target
        previous=target.read_bytes() if target.exists() else None
        if previous is not None and sha(previous)!=sha(data) and sha(previous)!=source.get(name):
            raise ValueError('Locally changed target requires merge: '+name)
        original[name]=previous
    backup=PROJECT/'output/local-install'/datetime.now().strftime('%Y%m%d-%H%M%S')
    backup.mkdir(parents=True,exist_ok=False)
    with zipfile.ZipFile(backup/'original-files.zip','w',zipfile.ZIP_DEFLATED) as z:
        for n,v in original.items():
            if v is not None:z.writestr(n,v)
    info=dict(instance=str(INSTANCE),release_url=receipt['url'],package_sha256=sha(package.read_bytes()),
        original={n:sha(v) if v is not None else None for n,v in original.items()},
        installed=manifest['files'],status='backup_complete',options_changed=False)
    log=backup/'installation.json'
    def save():log.write_text(json.dumps(info,ensure_ascii=False,indent=2),encoding='utf-8')
    save();written=[]
    try:
        for n,data in files.items():
            if data==original[n]:continue
            target=targets[n]
            if (target.read_bytes() if target.exists() else None)!=original[n]:raise ValueError('Target changed after preflight')
            target.parent.mkdir(parents=True,exist_ok=True)
            written.append(n);target.write_bytes(data)
        assert all(sha(targets[n].read_bytes())==manifest['files'][n] for n in files)
        info['status']='installed_and_hash_verified';info['written_files']=len(written);save()
    except BaseException:
        for n in reversed(written):
            if original[n] is None:targets[n].unlink(missing_ok=True)
            else:targets[n].write_bytes(original[n])
        info['status']='rolled_back';save();raise
    print(json.dumps(dict(status=info['status'],verified_files=len(files),backed_up_files=sum(v is not None for v in original.values()),
                         new_files=sum(v is None for v in original.values()),backup=str(backup),options_changed=False),ensure_ascii=False))


if __name__=='__main__':main()
