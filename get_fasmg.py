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

# CE-Programming/toolchain revision the includes come from. ce-libify emits
# fasmg sources, and toolchain master moved its libraries to GAS after v15.0
# (include_library.inc and ti84pceg.inc are gone there), so the default is the
# last fasmg release. Override with --toolchain-ref or CE_TOOLCHAIN_REF to
# match the toolchain your build pins.
DEFAULT_TOOLCHAIN_REF = "v15.0"
TOOLCHAIN_RAW = "https://raw.githubusercontent.com/CE-Programming/toolchain"

# Files taken from the toolchain, all resolved against one ref.
TOOLCHAIN_INCLUDES = (
    "library.inc",
    "include_library.inc",
    "ti84pceg.inc",
)

# Everything else, pinned per-source since it comes from other repositories.
OTHER_SOURCES = {
    "ez80.alm": "https://raw.githubusercontent.com/jacobly0/fasmg-ez80/main/ez80.alm",
    "commands.alm": "https://raw.githubusercontent.com/jacobly0/fasmg-ez80/main/commands.alm",
    "ez80.inc": "https://raw.githubusercontent.com/jacobly0/fasmg-ez80/main/ez80.inc",
    "tiformat.inc": "https://raw.githubusercontent.com/cagscalclabs/cryptx/dev/include/tiformat.inc",
}


def macro_sources(toolchain_ref=DEFAULT_TOOLCHAIN_REF):
    """Every macro file to fetch, with the toolchain ones at `toolchain_ref`."""
    sources = dict(OTHER_SOURCES)
    for name in TOOLCHAIN_INCLUDES:
        sources[name] = f"{TOOLCHAIN_RAW}/{toolchain_ref}/src/include/{name}"
    return sources


# Kept so existing importers of MACRO_SOURCES still work.
MACRO_SOURCES = macro_sources()

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


def install_macros(destination, toolchain_ref=DEFAULT_TOOLCHAIN_REF):
    for name, url in macro_sources(toolchain_ref).items():
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
    parser.add_argument(
        "--toolchain-ref",
        default=os.environ.get("CE_TOOLCHAIN_REF", DEFAULT_TOOLCHAIN_REF),
        help="CE-Programming/toolchain branch or tag to take the includes "
             "from (default: $CE_TOOLCHAIN_REF, else "
             f"{DEFAULT_TOOLCHAIN_REF}). Set this to the same revision the "
             "rest of your build pins.")
    args = parser.parse_args()

    os.makedirs(args.bin, exist_ok=True)
    os.makedirs(args.vendor, exist_ok=True)

    name = ARCHIVE_MEMBERS[platform.system()][1]
    if args.force or not os.path.exists(os.path.join(args.bin, name)):
        install_fasmg(args.bin)
    else:
        print("[get-fasmg] fasmg already present")

    sources = macro_sources(args.toolchain_ref)
    missing = [n for n in sources if not os.path.exists(os.path.join(args.vendor, n))]
    if args.force or missing:
        if args.toolchain_ref != DEFAULT_TOOLCHAIN_REF:
            print(f"[get-fasmg] toolchain includes from {args.toolchain_ref}")
        install_macros(args.vendor, args.toolchain_ref)
    else:
        print("[get-fasmg] macro packages already present")

    return 0


if __name__ == "__main__":
    sys.exit(main())
