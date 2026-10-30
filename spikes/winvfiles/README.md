# winvfiles - Windows virtual files on the clipboard (P0-SP2-002)

## Purpose

Decide whether D-11 holds on Windows: a hand-built `IDataObject` (pure Go, `spikes/internal/wincom`) placed with
`OleSetClipboard` that offers `FileGroupDescriptorW` plus one `FileContents` `IStream` per file, so Explorer pulls
the bytes only when the user pastes and shows its own progress UI.

The spike offers this tree (content is `byte(offset*31+7)`, served by a throttled synthetic source):

| lindex | path | kind | size |
|---|---|---|---|
| 0 | `demo` | directory | 0 |
| 1 | `demo\a.bin` | file | 200 MiB |
| 2 | `b.txt` | file | 10 KiB |

Formats offered by the data object (`EnumFormatEtc`):

| Format (registered name) | tymed | content |
|---|---|---|
| `FileGroupDescriptorW` | `TYMED_HGLOBAL` | `FILEGROUPDESCRIPTORW`, flags `FD_ATTRIBUTES\|FD_FILESIZE\|FD_WRITESTIME\|FD_PROGRESSUI\|FD_UNICODE` per entry |
| `FileContents` | `TYMED_ISTREAM` | one new read-only `IStream` per `GetData` call, selected by `lindex` (directories and out-of-range answer `DV_E_LINDEX`) |
| `Preferred DropEffect` | `TYMED_HGLOBAL` | `DROPEFFECT_COPY` |

`SetData` accepts `Performed DropEffect` and `Paste Succeeded` (a DWORD drop effect each) and logs them; any other
format is refused with `DV_E_FORMATETC` and logged.

## Build

From the repository root:

```
go -C spikes build -o bin/winvfiles.exe ./winvfiles
```

## Run

Run from `spikes\`. The program puts the files on the clipboard and waits for Ctrl+C. Every console line is a
`log/slog` text record with a timestamp.

```
.\bin\winvfiles.exe
.\bin\winvfiles.exe -rate 4 -latency 500ms
.\bin\winvfiles.exe -blockread 3s
.\bin\winvfiles.exe -flush
```

Flags:

- `-rate <MiB/s>` per-stream throughput (default 16; 0 = unthrottled)
- `-latency <duration>` delay before the first byte of every stream (default 200ms)
- `-blockread <duration>` the very first `Read` of the whole run blocks for this long (default off)
- `-flush` call `OleFlushClipboard` on Ctrl+C. Without it the program just releases its own reference and exits, which
  is the "no flush" case.

## What to do and observe

Use a fresh Explorer window as the paste target (a local NTFS folder, e.g. `C:\Temp\paste`). Copy nothing else while a
run is active: another copy replaces the clipboard and the program logs `clipboard ownership lost`.

1. Plain paste: `.\bin\winvfiles.exe`, then paste (Ctrl+V) into the folder.
   - Does Explorer's copy dialog appear, does it show names, item count and total size (about 200 MB) from the start?
   - Does the speed shown match `-rate`? Do `demo\a.bin` (200 MiB) and `b.txt` (10 KiB) arrive with correct sizes and
     with the modification time of the program start (check Properties)? Is `demo` created as a folder?
   - In the log: which calls arrive (`GetData` for which format and `lindex`, `SetData` for `Performed DropEffect` and
     `Paste Succeeded`), in which order, and with which `tymed`/`aspect`? Is `FileContents` ever requested for
     `lindex=0` (the directory)?
   - The `stream created, nothing read yet` line must come before any content is served; `first_byte_after` in the
     `stream released` line is the time to first byte.
2. Overwrite: paste into the same folder a second time and choose "Replace" in the conflict dialog. Does the dialog show
   the size and date of the source file (`FD_FILESIZE`, `FD_WRITESTIME`)?
3. Cancel: paste again with `-rate 4` and press Cancel in Explorer's dialog after about 10 s. Do the reads stop at once
   (no further log activity)? Is the `stream released` line `completed=false` with `served` well below `size`? How
   long after pressing Cancel does it appear?
4. Blocking Read: `.\bin\winvfiles.exe -blockread 3s`, paste, and during the 3 s stall try to move the Explorer window,
   open another folder, right-click, and type in the address bar. Is Explorer's UI frozen, only the copy dialog, or
   nothing? Does the paste complete correctly afterwards?
5. Exit without flush: `.\bin\winvfiles.exe` (no `-flush`), wait for the `virtual files are on the clipboard` line,
   press Ctrl+C, then paste into Explorer. What happens (nothing, an error dialog, an old clipboard content, an empty
   paste)? Record the exact behaviour and whether a clipboard viewer still lists any formats.
6. Exit with flush: `.\bin\winvfiles.exe -flush`, press Ctrl+C, wait for it to return, then paste into Explorer. What
   happens now? Watch the log during the flush: which `GetData` calls does `OleFlushClipboard` make, for which
   `tymed`, and does it read `FileContents` content (a flush that renders the 200 MiB stream would show many reads and
   take about 12 s at the default rate)?
7. Ownership: while the program runs, copy some text in any application. The program must log
   `clipboard ownership lost` within about 1 s.
8. Elevated Explorer (optional): repeat step 1 with the paste target opened by an elevated Explorer
   ("Run as administrator" on a File Explorer instance, or an elevated file manager) and record the result.

Logging limits to keep in mind when reading the output: the `Formats` line is printed whenever the data object
consults the format list, that is for `EnumFormatEtc`, for `QueryGetData` (the queried FORMATETC is not visible to the
data source) and as the pre-check of every `GetData`; a `GetData` for a format the object does not offer (for
example `Shell IDList Array`) is answered with `DV_E_FORMATETC` by the COM layer and does not reach the log.

## Paste back

For every numbered step: the complete console output of the run from the first line to the `file accounting` /
`paste outcomes` lines after Ctrl+C, plus what Explorer showed (copy dialog text, speeds, any error dialog, whether the
UI stayed responsive in step 4, what the paste produced in steps 5 and 6). Also paste the Windows version (`winver`)
and whether Explorer was elevated.
