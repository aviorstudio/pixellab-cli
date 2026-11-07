#!/usr/bin/env python3
"""Materialize native runtime filenames from tracked functional inputs."""
import tomllib
from pathlib import Path
import os
import sys

ROOT = Path(__file__).resolve().parents[1]

def prepare():
    manifest = ROOT / '.github/runtime-sources.toml'
    if manifest.is_symlink() or not manifest.is_file() or manifest.stat().st_size > 262144:
        raise ValueError('runtime manifest must be a bounded regular TOML source')
    declaration = tomllib.loads(manifest.read_text(encoding='utf-8'))
    if set(declaration) != {'sources'} or not isinstance(declaration['sources'], list) or len(declaration['sources']) > 128:
        raise ValueError('runtime manifest requires one bounded sources array')
    records = declaration['sources']
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
