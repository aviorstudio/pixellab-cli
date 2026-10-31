"""Reject release executables that require an extra C runtime installation."""
from pathlib import Path
import struct
import subprocess


def verify_linkage(path, system):
    if system == 'Linux':
        assert 'INTERP' not in subprocess.check_output(['readelf', '-l', str(path)], text=True)
        assert '(NEEDED)' not in subprocess.check_output(['readelf', '-d', str(path)], text=True)
    elif system == 'Windows':
        data = Path(path).read_bytes()
        assert data[:2] == b'MZ'
        pe = struct.unpack_from('<I', data, 0x3c)[0]
        assert data[pe:pe+4] == b'PE\0\0'
        count = struct.unpack_from('<H', data, pe+6)[0]
        optional_size = struct.unpack_from('<H', data, pe+20)[0]
        optional = pe+24
        assert struct.unpack_from('<H', data, optional)[0] == 0x20b
        sections = optional+optional_size
        def offset(rva):
            for index in range(count):
                virtual_size, address, raw_size, raw_offset = struct.unpack_from('<IIII', data, sections+index*40+8)
                if address <= rva < address+max(virtual_size, raw_size):
                    return raw_offset+rva-address
            raise AssertionError('PE import address has no section')
        imports = struct.unpack_from('<I', data, optional+112+8)[0]
        if not imports:
            return
        cursor = offset(imports)
        while any(data[cursor:cursor+20]):
            name = offset(struct.unpack_from('<I', data, cursor+12)[0])
            library = data[name:data.index(b'\0', name)].decode('ascii').lower()
            assert not library.startswith(('vcruntime', 'msvcp', 'api-ms-win-crt-')), library
            cursor += 20
