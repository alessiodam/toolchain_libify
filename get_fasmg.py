#!/usr/bin/env python3
import argparse
import io
import os
import platform
import re
import shutil
import stat
import sys
import time
import urllib.error
import urllib.request
import zipfile

FASMG_SITE = "https://flatassembler.net"
FASMG_PAGE = FASMG_SITE + "/download.php"

MACRO_SOURCES = {
    "ez80.alm": "https://raw.githubusercontent.com/jacobly0/fasmg-ez80/main/ez80.alm",
    "commands.alm": "https://raw.githubusercontent.com/jacobly0/fasmg-ez80/main/commands.alm",
    "ez80.inc": "https://raw.githubusercontent.com/jacobly0/fasmg-ez80/main/ez80.inc",
    "library.inc": "https://raw.githubusercontent.com/CE-Programming/toolchain/master/src/include/library.inc",
    "include_library.inc": "https://raw.githubusercontent.com/CE-Programming/toolchain/master/src/include/include_library.inc",
    "ti84pceg.inc": "https://raw.githubusercontent.com/CE-Programming/toolchain/master/src/include/ti84pceg.inc",
    "tiformat.inc": "https://raw.githubusercontent.com/cagscalclabs/cryptx/dev/include/tiformat.inc",
}

ARCHIVE_MEMBERS = {
    "Windows": ("fasmg.exe", "fasmg.exe"),
    "Linux": ("fasmg.x64", "fasmg"),
    "Darwin": ("source/macos/x64/fasmg", "fasmg"),
}


RETRIES = 4


def fetch(url):
    for attempt in range(1, RETRIES + 1):
        try:
            with urllib.request.urlopen(url, timeout=60) as response:
                return response.read()
        except (urllib.error.URLError, TimeoutError, ConnectionError) as error:
            if attempt == RETRIES:
                raise SystemExit("failed to fetch %s: %s" % (url, error))
            delay = 2 ** attempt
            print("[get-fasmg] %s failed (%s), retrying in %ds" % (url, error, delay))
            time.sleep(delay)


def fasmg_archive_url():
    page = fetch(FASMG_PAGE).decode("utf-8", "replace")
    match = re.search(r'href="(fasmg\.[a-z0-9]+\.zip)"', page)
    if not match:
        raise SystemExit("could not find a fasmg archive link on " + FASMG_PAGE)
    return FASMG_SITE + "/" + match.group(1)


def install_fasmg(destination):
    system = platform.system()
    if system not in ARCHIVE_MEMBERS:
        raise SystemExit("unsupported platform: " + system)

    member, name = ARCHIVE_MEMBERS[system]
    target = os.path.join(destination, name)

    url = fasmg_archive_url()
    print("[get-fasmg] " + url)
    archive = zipfile.ZipFile(io.BytesIO(fetch(url)))

    with archive.open(member) as source, open(target, "wb") as out:
        shutil.copyfileobj(source, out)

    os.chmod(target, os.stat(target).st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
    print("[get-fasmg] " + target)
    return target


def install_macros(destination):
    for name, url in MACRO_SOURCES.items():
        target = os.path.join(destination, name)
        print("[get-fasmg] " + name)
        with open(target, "wb") as out:
            out.write(fetch(url))


def main():
    root = os.path.dirname(os.path.abspath(__file__))

    parser = argparse.ArgumentParser(
        description="Download fasmg and the ez80 macro packages ce-libify needs.")
    parser.add_argument("--bin", default=os.path.join(root, "build"))
    parser.add_argument("--vendor", default=os.path.join(root, "build", "vendor"))
    parser.add_argument("--force", action="store_true")
    args = parser.parse_args()

    os.makedirs(args.bin, exist_ok=True)
    os.makedirs(args.vendor, exist_ok=True)

    name = ARCHIVE_MEMBERS[platform.system()][1]
    if args.force or not os.path.exists(os.path.join(args.bin, name)):
        install_fasmg(args.bin)
    else:
        print("[get-fasmg] fasmg already present")

    missing = [n for n in MACRO_SOURCES if not os.path.exists(os.path.join(args.vendor, n))]
    if args.force or missing:
        install_macros(args.vendor)
    else:
        print("[get-fasmg] macro packages already present")

    return 0


if __name__ == "__main__":
    sys.exit(main())
