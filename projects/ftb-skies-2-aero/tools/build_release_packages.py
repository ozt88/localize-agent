"""Build client/server release assets from the verified core artifact only."""
import hashlib
import json
from pathlib import Path
import zipfile

PROJECT=Path(__file__).resolve().parents[1]
CORE=PROJECT/'output/pack-translation-20261011'
OUT=PROJECT/'output/releases/ftb-skies-2-aero-1.13.0-ko-v1'
TAG='ftb-skies-2-aero-1.13.0-ko-v1'


def sha(data):return hashlib.sha256(data).hexdigest()


def main():
    manifest=json.loads((CORE/'package-manifest.json').read_text(encoding='utf-8'))
    package=CORE/'ftb-skies-2-aero-1.13.0-ko-core.zip'
    assert sha(package.read_bytes())==manifest['package_sha256']
    with zipfile.ZipFile(package) as z:
        assert z.testzip() is None
        files={n:z.read(n) for n in z.namelist()}
    assert {n:sha(v) for n,v in files.items()}==manifest['files']
    assert all(not Path(n).is_absolute() and '..' not in Path(n).parts for n in files)
    OUT.mkdir(parents=True,exist_ok=True)
    assets=[]
    common='''FTB Skies 2: Aero 1.13.0 한국어 핵심 번역 v1 (사전 릴리즈)
대상: 모드팩 1.13.0 / Minecraft 1.21.1 / NeoForge 21.1.250
다른 모드팩 버전에는 설치하지 마세요. 이 파일은 원본 모드팩이 필요한 덮어쓰기 패치입니다.
게임/서버를 완전히 종료하고 해당 인스턴스의 config, kubejs, resourcepacks 폴더를 먼저 백업하세요.
ZIP 안의 config, kubejs 등 폴더를 인스턴스 루트에 병합하여 복사하세요.
폴더 전체를 삭제하거나 교체하지 말고 같은 경로의 패치 파일만 덮어쓰세요.
직접 수정한 동일 경로의 스크립트가 있으면 덮어쓰기 전에 변경사항을 병합해야 합니다.
업데이트 전 백업을 복원하세요. 제거 시 새로 추가한 파일은 삭제하고 기존 파일은 백업으로 복구하세요.

검증: 번역 편입, 원본 해시, 보호구문, JSON/SNBT 구조, JavaScript 구문 및 ZIP 무결성 검사 통과.
미검증: 실제 게임 화면과 전용 서버 동작. 서버 배포본과의 파일 일치도 별도 실증하지 않았습니다.
알려진 원본 문제: quest.2617A856093F36AB.quest_desc의 이미지 제어문 1개는 원본부터 닫힘 기호가 잘못되어 그대로 보존했습니다.
전체 모드/가이드 번역이 아니라 퀘스트와 모드팩 자체 문구의 핵심 패치입니다.
서버 스크립트 문구는 언어 선택과 무관하게 한국어로 표시됩니다.
모드 JAR, 월드, 사용자 설정, 인증정보, 내부 실행 로그는 포함하지 않습니다.
원본 모드팩 및 모드의 권리는 해당 제작자에게 있습니다. 비공식 한국어 패치입니다.
'''
    for side in ('client','server'):
        selected=files if side=='client' else {n:v for n,v in files.items() if n.startswith('kubejs/server_scripts/') or n=='config/ftbquests/quests/lang/ko_kr.snbt'}
        assert len(selected)==(23 if side=='client' else 13)
        instructions=(
            '\n클라이언트 설치\nCurseForge에서 FTB Skies 2 Aero 프로필의 폴더 열기를 사용해 인스턴스 루트를 찾으세요.\n'
            'config, kubejs, resourcepacks를 이 루트에 병합하세요. 이 ZIP 자체를 resourcepacks에 넣는 방식이 아닙니다.\n'
            '게임을 재시작하고 언어를 한국어(대한민국)로 설정하세요. 기존 FTB 리소스 팩은 활성화 상태를 유지하세요.\n'
            '싱글플레이는 이 클라이언트 패키지 하나로 구성됩니다. 멀티플레이는 서버 관리자가 서버 패키지도 설치해야 전체 서버 문구가 적용됩니다.\n'
            if side=='client' else
            '\n전용 서버 설치\n같은 버전 서버의 config와 kubejs 폴더가 있는 루트에 두 폴더를 병합한 뒤 서버를 재시작하세요.\n'
            '퀘스트 ko_kr.snbt 및 서버 스크립트 12개만 포함됩니다. 클라이언트 리소스는 포함하지 않습니다.\n'
            '접속자도 클라이언트 패키지를 설치하고 한국어를 선택해야 아이템명·그룹명 등 클라이언트 리소스까지 적용됩니다.\n')
        metadata=dict(pack='FTB Skies 2: Aero',pack_version='1.13.0',patch_version='v1',side=side,
            status='prerelease; offline validation complete; in-game validation pending',
            upstream_core_sha256=manifest['package_sha256'],files={n:sha(v) for n,v in selected.items()})
        output=OUT/f'ftb-skies-2-aero-1.13.0-ko-v1-{side}.zip'
        entries=dict(selected)
        entries['README-KO.txt']=(common+instructions).encode('utf-8')
        entries['translation-manifest.json']=json.dumps(metadata,ensure_ascii=False,indent=2).encode('utf-8')
        with zipfile.ZipFile(output,'w',zipfile.ZIP_DEFLATED) as z:
            for n,v in sorted(entries.items()):
                info=zipfile.ZipInfo(n,(2026,10,11,0,0,0));info.compress_type=zipfile.ZIP_DEFLATED
                z.writestr(info,v)
        with zipfile.ZipFile(output) as z:
            assert z.testzip() is None and set(z.namelist())==set(entries)
            assert all(z.read(n)==v for n,v in entries.items())
        assets.append(output)
    (OUT/'SHA256SUMS.txt').write_text(''.join(f'{sha(p.read_bytes())}  {p.name}\n' for p in assets),encoding='utf-8')
    notes='''## FTB Skies 2: Aero 1.13.0 한국어 핵심 번역

퀘스트 5,732개, 모드팩 언어 항목 2,459개, 스크립트 문구 27개를 처리·편입한 비공식 한국어 패치입니다.
Minecraft 1.21.1 / NeoForge 21.1.250 / 모드팩 **1.13.0 전용**입니다.

### 다운로드
- **일반 플레이어·싱글플레이:** `ftb-skies-2-aero-1.13.0-ko-v1-client.zip` — 패치 파일 23개.
- **전용 서버 관리자:** `ftb-skies-2-aero-1.13.0-ko-v1-server.zip` — 퀘스트 언어 파일과 서버 스크립트, 패치 파일 13개.
- `SHA256SUMS.txt` — 두 ZIP의 SHA-256 체크섬.
- GitHub의 자동 생성 `Source code` 다운로드는 설치용 번역 패치가 아닙니다.

### 설치
1. 원본 모드팩 1.13.0을 설치하고 게임/서버를 완전히 종료합니다.
2. 기존 `config`, `kubejs`, `resourcepacks`를 백업합니다.
3. 해당 ZIP을 풀어 인스턴스 루트에 폴더를 **병합**합니다. 폴더를 통째로 삭제·교체하지 마세요. 직접 변경한 스크립트는 먼저 병합하세요.
4. 게임/서버를 재시작하고 클라이언트 언어를 **한국어(대한민국)**로 설정합니다. 기존 FTB 리소스 팩을 활성화해 두세요.

클라이언트 ZIP 자체를 `resourcepacks`에 넣는 방식이 아닙니다. 싱글플레이는 클라이언트 패키지만 사용합니다.
멀티플레이는 서버에 서버 패키지, 각 접속자에게 클라이언트 패키지를 설치해야 전체 범위를 적용할 수 있습니다.
서버 스크립트 문구는 플레이어의 언어 설정과 무관하게 한국어로 표시됩니다.
제거/업데이트 전에는 기존 파일을 백업으로 복구하고 이 패치가 새로 추가한 파일을 제거하세요. 자세한 방법은 각 ZIP의 `README-KO.txt`에 있습니다.

### 검증 상태와 제한
번역 편입, 원본 해시, 자리표시자·보호구문, SNBT/JSON 구조, JavaScript 구문 및 ZIP 무결성을 검증했습니다.
**게임 내 화면·전용 서버 실증은 아직 하지 않아 사전 릴리즈로 제공합니다.** 전용 서버 배포본과의 파일 일치도 아직 실증하지 않았습니다.
원본의 `quest.2617A856093F36AB.quest_desc` 이미지 제어문 1개는 닫힘 기호 오류를 그대로 보존했습니다.
전체 개별 모드 및 가이드의 완전한 한국어화는 이번 범위에 포함하지 않습니다.

원본 모드팩과 모드의 권리는 각 제작자에게 있습니다. 모드 JAR, 월드, 개인정보 및 내부 실행 로그는 포함하지 않았습니다.
구조 참고: [FTB Quests 변경 기록](https://github.com/FTBTeam/FTB-Quests/blob/main/CHANGELOG.md), [KubeJS 폴더 구조](https://kubejs.com/wiki/folder-structure).
'''
    (OUT/'release-notes.md').write_text(notes,encoding='utf-8')
    print(json.dumps(dict(directory=str(OUT),assets=[p.name for p in assets],patch_files=dict(client=23,server=13)),ensure_ascii=False))


if __name__=='__main__':main()
