#!/usr/bin/env python3
"""Inspect Windows x64 native imports without claiming Windows execution.

The allowlist contains Windows system DLLs; Visual C++ runtime DLLs must be
bundled. Follow normal and delayed imports from declared entry points.
"""
import argparse
import json
from pathlib import Path
import struct

SYSTEM = set("""advapi32 avrt avicap32 bcrypt bcryptprimitives cabinet cfgmgr32 clbcatq
combase comctl32 comdlg32 credui crypt32 cryptbase cryptnet cryptsp cryptui
d2d1 d3d11 d3d9 dbghelp dnsapi dsound dwmapi dwrite dxgi dxva2 gdi32 gdiplus
hid imagehlp imm32 iphlpapi kernel32 kernelbase mpr msasn1 msimg32 msvcrt
ncrypt netapi32 normaliz ntdll ole32 oleacc oleaut32 opengl32 powrprof propsys
psapi rasapi32 rpcrt4 secur32 setupapi shell32 shlwapi sspicli ucrtbase urlmon
user32 userenv usp10 uxtheme version winhttp wininet winmm winspool.drv
wintrust wlanapi wldap32 ws2_32 wtsapi32 wsdapi winusb olepro32
httpapi oledlg mfplay mfreadwrite mfplat wsock32 wer fontsub
esent wevtapi dcomp dhcpcsvc msi activeds d3dcompiler_47 bthprops.cpl uiautomationcore pdh ndfapi mmdevapi mf d3d12""".split())
SYSTEM = {name if "." in name else name + ".dll" for name in SYSTEM}


def is_anycpu_assembly(path):
    """Pure IL without 32BITREQUIRED is platform-neutral, not x86 native code."""
    data = path.read_bytes()
    if len(data) < 64 or data[:2] != b"MZ": return False
    pe = struct.unpack_from("<I", data, 60)[0]
    if pe + 24 > len(data) or data[pe:pe+4] != b"PE\0\0": return False
    machine, count = struct.unpack_from("<HH", data, pe+4)
    size = struct.unpack_from("<H", data, pe+20)[0]
    optional = pe + 24
    if optional + size > len(data) or size < 224: return False
    if machine != 0x14c or struct.unpack_from("<H", data, optional)[0] != 0x10b: return False
    rva = struct.unpack_from("<I", data, optional + 96 + 14*8)[0]
    if not rva: return False
    for i in range(count):
        section = optional + size + i*40
        if section + 40 > len(data): return False
        virtual_size, address, raw_size, start = struct.unpack_from("<IIII", data, section+8)
        if address <= rva < address + max(virtual_size, raw_size):
            position = start+rva-address+16
            if position+4 > len(data): return False
            flags = struct.unpack_from("<I", data, position)[0]
            return bool(flags & 1) and not bool(flags & (2 | 16))
    return False


def imports(path):
    data = path.read_bytes()
    def unpack(fmt, offset):
        return struct.unpack_from(fmt, data, offset)
    if data[:2] != b"MZ" or len(data) < 64:
        raise ValueError(f"not PE: {path}")
    pe, = unpack("<I", 60)
    if data[pe:pe+4] != b"PE\0\0":
        raise ValueError(f"invalid PE signature: {path}")
    machine, count = unpack("<HH", pe+4)
    optional_size, = unpack("<H", pe+20)
    optional = pe+24
    magic, = unpack("<H", optional)
    if machine != 0x8664 or magic != 0x20B:
        raise ValueError(f"not Windows x64: {path}")
    base, = unpack("<Q", optional+24)
    sections = [unpack("<IIII", optional+optional_size+i*40+8) for i in range(count)]
    def offset(rva):
        for virtual_size, address, raw_size, raw_start in sections:
            if address <= rva < address + max(virtual_size, raw_size):
                result = raw_start+rva-address
                if result >= len(data): break
                return result
        raise ValueError(f"invalid RVA in {path}")
    def name(rva):
        start = offset(rva)
        end = data.find(b"\0", start, start+512)
        if end < 0: raise ValueError(f"invalid DLL name: {path}")
        return data[start:end].decode("ascii").lower()
    result = set()
    directories, = unpack("<I", optional+108)
    for index, stride in ((1, 20), (13, 32)):
        if directories <= index: continue
        rva, size = unpack("<II", optional+112+index*8)
        if not rva: continue
        start = offset(rva)
        for i in range(min(size//stride+1, 4096)):
            record = unpack("<"+"I"*(stride//4), start+i*stride)
            if not any(record): break
            address = record[3] if index == 1 else record[1]
            if index == 13 and not record[0] & 1: address -= base
            result.add(name(address))
        else: raise ValueError(f"unterminated import table: {path}")
    return result


def check(root, entries):
    pending = [(root/entry).resolve() for entry in entries]
    launch_dirs = {path.parent for path in pending}
    directory_index = {}
    def files(directory):
        if directory not in directory_index:
            directory_index[directory] = {p.name.lower(): p for p in directory.iterdir()}
        return directory_index[directory]
    fallback = {}
    for directory in sorted(launch_dirs):
        for name, candidate in files(directory).items(): fallback.setdefault(name, candidate)
    visited, missing = {}, []
    while pending:
        path = pending.pop()
        if path in visited: continue
        if root not in path.parents: raise ValueError("entry escapes bundle")
        dependencies = imports(path)
        visited[path] = sorted(dependencies)
        for dependency in dependencies:
            if dependency in SYSTEM or dependency.startswith(("api-ms-win-", "ext-ms-win-")): continue
            candidate = files(path.parent).get(dependency) or fallback.get(dependency)
            if not candidate:
                missing.append({"file": str(path.relative_to(root)), "dependency": dependency})
            else:
                pending.append(candidate.resolve())
    return {"architecture": "windows-x64", "native_execution_verified": False, "files_checked": len(visited), "windows_media_features": sorted({dependency for deps in visited.values() for dependency in deps if dependency in {"mfplay.dll", "mfreadwrite.dll", "mfplat.dll"}}), "missing": missing}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("root", type=Path)
    parser.add_argument("entry", nargs="+")
    args = parser.parse_args()
    result = check(args.root.resolve(), args.entry)
    print(json.dumps(result, indent=2))
    if result["missing"]: raise SystemExit(1)


if __name__ == "__main__": main()
