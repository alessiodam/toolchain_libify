# ce-libify
Build a LibLoad library for the TI-84 Plus CE out of ordinary C sources.

The CE C toolchain builds programs. LibLoad libraries are a different thing: a
single fasmg module with no linker, no libc and no BSS heap, normally written in
assembly by hand. `ce-libify` takes a directory of C files and produces the two
files a library ships as:
- `NAME.8xv`, the appvar you send to the calculator
- `NAME.lib`, the stub other projects link against

## Install
Needs Go 1.26 and Python 3 (Python only to download fasmg).

```
git clone https://github.com/alessiodam/toolchain_libify
cd toolchain_libify
make install
```

That builds the tool, downloads fasmg and the fasmg ez80 macro packages, then
copies all of it into your CEdev install:
```
$CEDEV/bin/ce-libify
$CEDEV/bin/fasmg
$CEDEV/meta/libify/*.inc, *.alm
```

`make install` reads `cedev-config --prefix` to find CEdev, so it lands in
whichever toolchain is on your PATH. Override it with `make install CEDEV=/path/to/CEdev`.
`make uninstall` removes all of it again.

## Example
A library called `DEMO` with two functions, checked in under [`demo/`](demo).
Build it:
```
ce-libify build --name DEMO --src src --headers . --exports exports.txt
```

You get `bin/DEMO.8xv` and `bin/DEMO.lib`. The demo also carries a
[`ce-libify.toml`](demo/ce-libify.toml), so running `ce-libify build` inside
`demo/` does the same with no flags.

Use it from a normal CE program by copying `DEMO.lib` into
`$CEDEV/lib/libload/` and `demo.h` into `$CEDEV/include/` (or another method if you prefer).

Send `DEMO.8xv` to the calculator alongside the program. LibLoad loads it when
the program starts.

## exports.txt
This file is the library's ABI. LibLoad resolves imports by ordinal, which is
just the position in this list, so the file is append only. Adding a line at the
end is safe. Reordering or deleting a line breaks every program already built
against a released `.8xv`.

Blank lines and lines starting with `#` are ignored.
```
ce-libify exports --exports exports.txt --header demo.h
```

reports any drift between the list and what your headers declare. Add
`--prefix demo_` to consider only declarations with a given prefix, which is
useful when a header also declares things you do not export.

## ce-libify.toml
Every command reads `ce-libify.toml` from the current directory when it exists.
Its keys are the command line flags, so a project stops repeating them:
```toml
name = "DEMO"
src = "src"
headers = "."
exports = "exports.txt"
header = ["demo.h"]
depend = ["usbdrvce"]
include = ["src/include"]
define = ["DEMO_DEBUG=1"]
exclude = ["contrib", "*/main.c"]
```

A flag given on the command line wins over the manifest. Keys are spelled
like the flags, `lib-version` included. Apart from the `[app]` and `[[step]]`
tables below, a key that matches no flag is an error, so a typo does not go
unnoticed. Sources are collected from
every directory under `src`; `exclude` takes globs relative to it, and a
matching directory is skipped as a whole.

### Steps
`[[step]]` entries run in order after a successful `build`, from the project
directory, with `CEDEV` set and `$CEDEV/bin` first on `PATH`. Use them for
project specific work such as generating release headers:
```toml
[[step]]
run = ["python3", "tools/make_headers.py"]
```

## Application libraries
A LibLoad appvar holds at most about 64 KB. A bigger library, lwIP-CE for
example, lives in a flash application instead, and a small LibLoad library
forwards calls into it. An `[app]` table switches `build` to that layout:
```toml
name = "DAPP"
exports = "exports.txt"
header = ["demoapp.h"]

[app]
name = "DemoApp"
init = "dapp_init"
```

`build` then produces:
- `bin/DAPP.0.8xv`, `bin/DAPP.1.8xv` and so on, the application split into
  appvars
- `bin/INSTALL.8xp`, the [app_tools](https://github.com/CE-Programming/app_tools)
  installer that writes those appvars to flash
- `bin/DAPP.8xv` and `bin/DAPP.lib`, the LibLoad library and its stub

The application is compiled with the toolchain's own `makefile.mk`, so every
source under `src`, C and `.s` assembly alike, is built the way a CE program
would be. `exports.txt` becomes an export table placed at offset `0x40` of the
application. The LibLoad library holds one `jp` per export. A program calls
`dapp_start_with_crt(malloc, free, realloc)` once, which finds the
application, checks the table, points every `jp` at the real function and
calls the `init` function with the import table. `init` has to be exported and
has the signature `bool init(const void *imports, size_t length)`. It is the
place to set up `.data` and `.bss`, since the application's own startup code
never runs. `dapp_get_start_errno()` and `dapp_get_start_errstring()` explain
a failure, and `dapp_is_newer()` reports an application newer than the stub.
These four functions come first in the library, ahead of `exports.txt`, so
their ordinals never move. To keep the slots of an existing release, list them
in `exports.txt` instead and they take the listed position. They get no entry
in the application's export table.

The import table starts with the three allocator pointers, followed by one
pointer for every name in `imports`, in order. Each name must be exported by a
library in `depend`, which is how the application reaches usbdrvce, fileioc
and others.

Keys of `[app]`:
| key | default | meaning |
| --- | --- | --- |
| `name` | required | application name, as the installer and `FindAppStart` see it |
| `init` | required | exported function the library calls once the table is patched |
| `imports` | none | library functions placed in the import table |
| `linker_script` | generated | linker script to use instead of the generated one, which must place `.fn_exports` at offset `0x40` |
| `split` | 65200 | appvar size the application is split into |
| `installer` | app_tools on GitHub | git URL of app_tools |
| `installer_ref` | a pinned commit | commit to build the installer from |
| `installer_patches` | none | patches applied to app_tools once, after cloning |

`[app.make]` passes variables to `makefile.mk` verbatim, which is where
`CFLAGS`, `ICON`, `BSSHEAP_HIGH`, `LTO` and the like go. `include`, `define`
and `optimize` only apply to plain LibLoad libraries. A full example lives in
[`demoapp/`](demoapp).

`emit` and `deps` are not available for application libraries.

## Depending on another library
A LibLoad library can use another one. Pass `--depend` and the dependency is
recorded in your appvar, so applications never have to know about it:
```
ce-libify build --name DEMO --src src --headers . --exports exports.txt --depend usbdrvce
```

`--depend` takes a name found in `$CEDEV/lib/libload`, or a path to a `.lib`
file. It is repeatable. The tool also rewrites the call sites for you: the
compiler emits `_usb_Init` for a C call, while the dependency stub defines
`usb_Init`, and the two are matched up automatically.

## Commands
| command | what it does |
| --- | --- |
| `build` | compile, rewrite and assemble into `.8xv` plus `.lib` |
| `emit` | write the generated assembly only, no fasmg |
| `deps` | report how every external symbol gets resolved |
| `exports` | check `exports.txt` against your headers |
| `package` | stage a release zip of the appvar, stub and headers |
| `install` | copy your built `.lib` and headers into CEdev |
| `clean` | remove the object and output directories |
| `selfinstall` | install or remove ce-libify itself, used by `make install` |

Run `ce-libify <command> --help` for the flags, `ce-libify --version` for the
build it came from.

## What the rewrite has to do
The C compiler does not emit something fasmg can assemble, and a LibLoad library
cannot link against anything. `ce-libify` bridges that gap:
1. Compile each source to LLVM bitcode, link the bitcode into one module, then
   emit one assembly file. Going through a single module keeps local labels
   unique and drops unused statics, which concatenating per file output cannot.
2. Drop `.section`, `.ident`, `public`, `extern` and `private`, which fasmg does
   not know.
3. Re-encode every string literal byte by byte. Clang writes raw NUL, high and
   quote bytes inside `db "..."`, and fasmg stops reading the file at the first
   NUL. Left alone this truncates the image and reports every later symbol as
   undefined, pointing at a line thousands away from the real problem.
4. Bind compiler helpers such as `__frameset0`, `__lmulu` and `_memcpy` to the
   TI-OS entry points in `ti84pceg.inc`, after checking each one exists.
5. Supply `__indcallhl` and `_atomic_load_32`, which the OS does not carry.
6. Match dependency call sites to the names `include_library` defines.
7. Emit the export aliases and the export table after the body, because fasmg
   resolves `:=` immediately.

Anything still unresolved stops the build and names the symbol and the line that
wants it, rather than letting fasmg fail somewhere unrelated:

```
$ ce-libify build --name DEMO --src src --headers . --exports exports.txt
unresolved symbols (a LibLoad library cannot link against libc):
  _snprintf	first used at lto.src:412
```

## Limits worth knowing
A LibLoad library has no BSS heap. Every static buffer is carried in the appvar
and copied into user RAM when the library loads, so a library with large arrays
produces a large `.8xv`. Prefer buffers supplied by the caller when size matters.

There is no libc. Functions the TI-OS exports are available, and `ce-libify deps`
will show you which ones got bound. Anything else, `snprintf` being the common
one, has to be avoided or written by hand.

## Development
```
make build
make test
make vet
make fmt
``` 

`make` uses a POSIX shell. On Windows run it from Git Bash or MSYS2.

## License
GNU GPLv3. See [LICENSE](LICENSE).
