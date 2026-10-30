# winclip - Windows delayed clipboard rendering (P0-SP2-003)

## Purpose

Decide whether D-10 holds on Windows for the non-file formats: a copy on one machine announces only the format list,
and the bytes are produced when a local application pastes. The spike offers five delayed formats from a message-only
window and renders each one on `WM_RENDERFORMAT` after an artificial delay:

| Format (name) | Id | Payload |
|---|---|---|
| `CF_UNICODETEXT` | 13 | UTF-16LE, CRLF line ends, 2-byte NUL terminator |
| `HTML Format` | registered | CF_HTML: `Version:1.0` header with byte offsets, UTF-8 body, NUL terminated |
| `Rich Text Format` | registered | RTF bytes, NUL terminated |
| `PNG` | registered | PNG bytes of a 16x8 image with an alpha ramp |
| `CF_DIBV5` | 17 | BITMAPV5HEADER (124 bytes), BI_BITFIELDS 32 bpp BGRA, bottom-up, sRGB |

With `-sensitive` the spike also offers `ExcludeClipboardContentFromMonitorProcessing` and
`CanIncludeInClipboardHistory` (DWORD 0), as a password manager does.

Every copy made by another application is logged through `WM_CLIPBOARDUPDATE` with the sequence number, the owner
window and the format list. The spike's own offers are recognised by the owner and ignored.

## Build

From the repository root:

```
go -C spikes build -o bin/winclip.exe ./winclip
```

## Run

Run from `spikes\`. Every console line is a `log/slog` text record. The program waits for Ctrl+C.

```
.\bin\winclip.exe
.\bin\winclip.exe -delay 2s
.\bin\winclip.exe -sensitive
```

Flags:

- `-delay <duration>` sleep before each `WM_RENDERFORMAT` is answered (default 2s, 0 = none). The sleep runs on the
  message-loop thread, which is what Microsoft Learn describes as the cost of delayed rendering.
- `-sensitive` also offer the two sensitivity formats.

## What to do and observe

Keep a second window open for each paste target.

1. Offer and log: start `.\bin\winclip.exe`. The log shows `offered delayed format` for each of the five formats.
   Copy a line of text in Notepad. Does the log show `clipboard update` with a new sequence number, the owner of
   Notepad (not the spike) and a format list starting with `CF_UNICODETEXT`? Are the spike's own offers reported as
   `clipboard update ignored: own delayed offer`?
2. Lazy paste per format: paste (Ctrl+V) into Notepad. Expect one `rendered delayed format` line with
   `elapsed` of about 2 s for `CF_UNICODETEXT`. Does Notepad pause for the delay? Repeat in WordPad or Word for the
   HTML Format (paste as formatted text), the RTF (Word and WordPad), and in Paint for `PNG` or `CF_DIBV5`. Record which
   format each application requested and whether the picture's transparent column (left edge, alpha 0) is transparent
   or white in Paint.
3. Text line endings: paste the text into Notepad and check that a blank line before `Line two` does not appear
   and that the emoji and accented characters survive.
4. Rendering on exit: copy something else in another application first (this replaces the spike's offers). Then
   press Ctrl+C and check the log. Then start the program again, do nothing, press Ctrl+C: the log must show
   `rendered delayed format at exit` for each format still delayed, and `WM_RENDERALLFORMATS: no delayed formats left`
   when all were rendered. After exit, paste into Notepad: does the text still paste? Does `CF_DIBV5` still paste into
   Paint?
5. Sensitivity formats: copy a password from a password manager. The `clipboard update` line must show
   `sensitivity_format=true` and list the two sensitivity format names. This is informational only: D-121 shares such
   content like any other. `-sensitive` does not affect this step, because the spike's own listener ignores its offers.

## Conventions the spike had to choose

- CF_HTML offsets: `StartFragment` is the first byte after `<!--StartFragment-->` and `EndFragment` is the first byte of
  `<!--EndFragment-->`. Microsoft Learn describes the fragment grammar with the markers but does not state which side
  of the marker the offset points to. Check that Word and browsers paste exactly the fragment and nothing else.
- CF_DIBV5 alpha: stored straight (non-premultiplied), the same representation as PNG. Microsoft Learn does not document
  the CF_DIBV5 alpha convention. Confirm in Paint (step 2) that the transparent column stays transparent.
- Text: `\n` is written as `\r\n` on the clipboard and read back as `\n`.

## What to paste back

- The complete log from start to Ctrl+C for one full run (steps 1 to 4).
- For each application in step 2: format requested, delay observed, and any visible defect.
- The step 5 log lines for the password-manager copy.
