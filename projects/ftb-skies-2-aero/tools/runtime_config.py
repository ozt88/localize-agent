"""Machine-local instance location; never store a user's absolute path in Git."""
import os
from pathlib import Path


def instance_path():
    configured=os.environ.get('FTB_AERO_INSTANCE','').strip()
    return Path(configured).expanduser().resolve() if configured else (
        Path.home()/'curseforge/minecraft/Instances/FTB Skies 2 Aero').resolve()
