#!/usr/bin/env python3
"""Materialize native runtime filenames from tracked functional inputs."""
import json
from pathlib import Path
import os
import sys

ROOT = Path(__file__).resolve().parents[1]

def prepare():
    records = json.loads((ROOT / '.github/runtime-sources.json').read_text())
    for row in records:
        source, target = ROOT / row['source'], ROOT / row['target']
        if not source.resolve().is_relative_to(ROOT.resolve()) or not target.parent.resolve().is_relative_to(ROOT.resolve()):
            raise ValueError('runtime path escapes checkout')
        content = source.read_bytes()
        target.parent.mkdir(parents=True, exist_ok=True)
        if target.is_symlink() or target.exists():
            expected = row.get('symlink')
            if expected is not None and target.is_symlink() and os.readlink(target) == expected:
                continue
            if expected is None and not target.is_symlink() and target.read_bytes() == content:
                continue
            raise ValueError('refusing to replace modified runtime input: ' + row['target'])
        if 'symlink' in row:
            target.symlink_to(row['symlink'])
        else:
            target.write_bytes(content)
    return len(records)

if __name__ == '__main__':
    print('Prepared', prepare(), 'runtime inputs.')
