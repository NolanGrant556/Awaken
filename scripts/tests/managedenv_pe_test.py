import importlib.util
from pathlib import Path
import struct
import tempfile
import unittest

SOURCE = Path(__file__).resolve().parents[1] / "check-managedenv-pe.py"
spec = importlib.util.spec_from_file_location("pe_validation", SOURCE)
checker = importlib.util.module_from_spec(spec)
spec.loader.exec_module(checker)


class ManagedAssemblyTests(unittest.TestCase):
    def test_32bit_required_assembly_is_not_anycpu(self):
        # Minimal PE/CLR headers model the distinction found in LibreOffice's
        # policy assemblies. This is a parser test, not Windows execution.
        data = bytearray(1024)
        data[:2] = b"MZ"
        struct.pack_into("<I", data, 60, 64)
        data[64:68] = b"PE\0\0"
        struct.pack_into("<HH", data, 68, 0x14c, 1)
        struct.pack_into("<H", data, 84, 224)
        struct.pack_into("<H", data, 88, 0x10b)
        struct.pack_into("<I", data, 88+96+14*8, 0x2000)
        struct.pack_into("<IIII", data, 88+224+8, 128, 0x2000, 128, 512)
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "policy.dll"
            for flags, accepted in [(9, True), (11, False), (17, False), (0, False)]:
                struct.pack_into("<I", data, 528, flags)
                path.write_bytes(data)
                self.assertEqual(checker.is_anycpu_assembly(path), accepted, flags)


if __name__ == "__main__": unittest.main()
