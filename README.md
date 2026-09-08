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

You get `bin/DEMO.8xv` and `bin/DEMO.lib`.

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
Apache License 2.0. See [LICENSE](LICENSE).
