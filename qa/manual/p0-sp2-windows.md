# SP2 Windows COM shell spikes - manual script

Item: P0-SP2-M01   Machines: Windows PC 1
Estimated time: 150 minutes   Needs admin: yes (optional: only the UIPI steps 16 and 25 need an elevated window or an elevated prompt)

Nothing in this script has been run by its author. It was written from the sources and READMEs of the four spikes (`spikes\winclip`, `spikes\winvfiles`, `spikes\windnd`, `spikes\giodrop`). The author did not start any spike binary.

## Preparation
1. Use a Windows 10 or Windows 11 PC and your normal (non-administrator) account. Open PowerShell and change into the repository root, the folder that contains `spikes`, `qa` and `plan.md`, for example `Set-Location D:\Development\projects\edgesync`.
2. Record the machine: press Win+R, type `winver`, press Enter, and note the Windows version and build. Note the display scale (Settings > System > Display), the number of monitors and which one is primary.
3. Set up the PowerShell window. These settings last only for this window:
   - `$env:CGO_ENABLED = "0"`
   - `$env:Path = "D:\Program Files\Go\bin;" + $env:Path`
   - `go version`

   Expect `go version go1.27.3 windows/amd64`. Any other output: stop and paste it back.
4. Create the scratch folders (paste targets and the drag source tree):
   - `New-Item -ItemType Directory -Force -Path C:\Temp\paste, C:\Temp\dnd, C:\Temp\dnd\sub, C:\Temp\dnd-out`
5. Fill the drag source folder:
   - `Set-Content -Path C:\Temp\dnd\one.txt -Value "one"`
   - `Set-Content -Path C:\Temp\dnd\two.txt -Value "two"`
   - `Set-Content -Path C:\Temp\dnd\sub\inner-1.txt -Value "inner 1"`
   - `Set-Content -Path C:\Temp\dnd\sub\inner-2.txt -Value "inner 2"`
   - In Explorer, select `one.txt` and `two.txt` in `C:\Temp\dnd`, right-click, choose Send to > Compressed (zipped) folder and name the archive `pack.zip`.

   Expect `C:\Temp\dnd` to contain `one.txt`, `two.txt`, `sub` and `pack.zip`.
6. Build the four binaries from the repository root. Each command prints nothing on success:
   - `go -C spikes build -o bin/winvfiles.exe ./winvfiles`
   - `go -C spikes build -o bin/winclip.exe ./winclip`
   - `go -C spikes build -o bin/windnd.exe ./windnd`
   - `go -C spikes build -o bin/giodrop.exe ./giodrop`

   `-C spikes` makes Go change into `spikes` first, so `bin/` means `spikes\bin`. Do not write `-o spikes/bin/...`: it would create `spikes\spikes\bin`. The `bin` folder is created by the first build. If a build fails, stop and paste the complete error text.
7. Check the binaries: `Get-ChildItem -Path .\spikes\bin -Filter *.exe`. Expect `giodrop.exe`, `winclip.exe`, `windnd.exe` and `winvfiles.exe`.
8. Working folders for the runs. Run `winclip`, `winvfiles` and `windnd` from the `spikes` folder (`Set-Location .\spikes`, then `.\bin\<name>.exe`). Run `giodrop` from the repository root (`Set-Location ..`, then `.\spikes\bin\giodrop.exe`).

Each step below starts a program in its own console window. Stop a program with Ctrl+C in its console before starting the next run of the same program, unless a step says otherwise.

## Steps

### Step 1 - winclip: start and offers
- Do: Run `.\bin\winclip.exe` (no flags) in the `spikes` folder. Do not copy or paste anything yet. Leave the console open.
- Expect: the startup line says `offered=5`, `delay=2s` and `sensitive=false`. Five `offered delayed format` lines follow, with `format` set to `CF_UNICODETEXT`, `HTML Format`, `Rich Text Format`, `PNG` and `CF_DIBV5`. A `clipboard update ignored: own delayed offer` line appears for the spike's own offer. No error line.
- Record: the complete console output from the first line to now, with the sequence number of the ignored update.

### Step 2 - winclip: paste into Notepad (CF_UNICODETEXT, delayed)
- Do: Open Notepad with an empty document. Click into it and press Ctrl+V. Note the time before the paste and the time the text appears.
- Expect: Notepad pauses for about 2 s, then shows two lines: `EdgeSync delayed rendering spike` and `Line two: é 世界 😀`. The text has no blank line between these two lines. The accented letter, the Chinese characters and the emoji are intact. The console shows `rendered delayed format` with `format=CF_UNICODETEXT`, `delay=2s` and `elapsed` of at least 2s.
- Record: the console lines of this step, the pause you measured, the pasted text as shown, and yes/no for "no blank line between the two lines" and "characters intact".
- Optional: close winclip with Ctrl+C, run `.\bin\winclip.exe -delay 0`, and paste into a new Notepad document. Expect no visible pause and `delay=0s` in the startup line. Record yes/no.

### Step 3 - winclip: paste into WordPad and Word (HTML Format and RTF)
- Do: If you ran the optional part of step 2, stop that winclip with Ctrl+C and start `.\bin\winclip.exe` again (default delay). Otherwise keep the winclip from step 1 running. Open WordPad, press Ctrl+V into an empty document. Then open a new blank Word document and press Ctrl+V. Then in Word press Ctrl+Alt+V (Paste Special) and choose `HTML Format`. Repeat with `Formatted Text (RTF)` if Word lists it.
- Expect: the text `EdgeSync delayed rendering spike` with `EdgeSync` in bold and `spike` in italic. Nothing else: no HTML tags, no lines starting with `Version:`, `StartHTML`, `StartFragment` or `<!--`, and no extra blank paragraphs. The console shows `rendered delayed format` lines for `HTML Format` and `Rich Text Format` as the applications request them.
- Record: which format each application requested (from the console), the pasted text as shown, and any stray characters verbatim.
- Open point (CF_HTML StartFragment): the spike places `StartFragment` after the `<!--StartFragment-->` marker. Confirm that Word pastes exactly the fragment and nothing else. Record the result for Word and for WordPad separately.

### Step 4 - winclip: paste into Paint (PNG or CF_DIBV5, alpha)
- Do: Open Paint with a new canvas and press Ctrl+V. The picture is 16 by 8 pixels. Zoom in until single pixels are visible (the zoom control at the bottom right of Paint, or Ctrl and the + key).
- Expect: a small gradient. Red rises from left to right and green rises from top to bottom. The left-most column is alpha 0 (transparent) and the alpha rises to fully opaque at the right edge. The console shows `rendered delayed format` for `PNG` or for `CF_DIBV5`, depending on which format Paint requested.
- Record: the format Paint requested (from the console), what the left-most column looks like (transparent, checkerboard, white or grey), and what the right-most column looks like.
- Open point (CF_DIBV5 alpha): the spike stores CF_DIBV5 alpha straight (not premultiplied), like PNG. Confirm in Paint that the transparent column stays transparent. If Paint requested `PNG` and not `CF_DIBV5`, write "CF_DIBV5 not exercised in Paint" and do not guess.

### Step 5 - winclip: a copy in another application replaces the offers
- Do: Click into Notepad, type `other text`, select it and press Ctrl+C. Optional: repeat with a copy in Word or in a browser.
- Expect: a `clipboard update` line with a new `sequence` number, an `owner_hwnd` that is not the spike's, and a format list of the copied content. Then a `delayed offer replaced by another copy` line with `unrendered_formats` set to the number of formats not yet pasted in this run.
- Record: the two log lines (or the optional extra lines) and the sequence numbers.
- Then close winclip with Ctrl+C.

### Step 6 - winclip: rendering on exit
- Do, run 6a: start `.\bin\winclip.exe`. Do not paste. Press Ctrl+C in its console.
- Expect, run 6a: five `rendered delayed format at exit` lines, one for each format, and the console returns to the prompt with no error line.
- Do, after run 6a: open Notepad and press Ctrl+V. Then open Paint and press Ctrl+V.
- Expect: the Notepad text pastes at once (no delay, because the format was rendered at exit). Paint pastes the picture if it requested `PNG` or `CF_DIBV5`.
- Do, run 6b: start `.\bin\winclip.exe` again. Paste into Notepad, then into WordPad or Word, then into Paint, so that as many formats as possible are pasted. Press Ctrl+C.
- Expect, run 6b: `rendered delayed format at exit` only for the formats that were not pasted, or `WM_RENDERALLFORMATS: no delayed formats left` if every format was already rendered.
- Record: the console output of runs 6a and 6b, the result of the two paste checks after run 6a, and which of the two exit messages appeared in 6b.

### Step 7 - winclip: sensitivity formats from a password manager
- Do: Start `.\bin\winclip.exe -sensitive`. Then copy a password from your password manager (or from any application that marks copied content as sensitive). If you have no password manager, write "not run" in the record. Press Ctrl+C when done.
- Expect: seven `offered delayed format` lines, including `ExcludeClipboardContentFromMonitorProcessing` and `CanIncludeInClipboardHistory`. After the password copy, the `clipboard update` line shows `sensitivity_format=true` and lists the two sensitivity format names. This result is informational only.
- Record: the offered lines, the `clipboard update` line for the password copy, and the name of the password manager (or "not run").

### Step 8 - winvfiles: plain paste (virtual files and Explorer progress)
- Do: Run `.\bin\winvfiles.exe` (defaults: rate 16 MiB/s, latency 200ms). Wait for the line `virtual files are on the clipboard; paste into Explorer, press Ctrl+C to exit`. Open Explorer on `C:\Temp\paste` and press Ctrl+V. Watch the copy dialog until it closes.
- Expect: the copy dialog shows the names, the item count and the total size (about 200 MB) from the start. The speed is close to 16 MB/s. Explorer creates the folder `demo`, the file `demo\a.bin` (200 MiB) and the file `b.txt` (10 KiB). Check Properties: the modified time of both files is the time you started winvfiles. The console shows `IDataObject call` lines, a `stream created, nothing read yet` line before the first byte, and `stream released` lines with `served`, `size`, `completed=true` and `first_byte_after`.
- Record: the console output from the first line to the paste completion, the copy dialog text, the speed shown, the sizes and times of the two files, and whether `demo` is a folder.

### Step 9 - winvfiles: overwrite (same run as step 8)
- Do: Without stopping winvfiles, press Ctrl+V again in `C:\Temp\paste`. In the conflict dialog choose Replace.
- Expect: the conflict dialog shows the size and the date of the source file (`FD_FILESIZE` and `FD_WRITESTIME`).
- Record: the text of the conflict dialog (size and date shown), and the console lines added by the second paste.

### Step 10 - winvfiles: cancel mid-way
- Do: Stop winvfiles with Ctrl+C. Run `.\bin\winvfiles.exe -rate 4 -latency 500ms`. Press Ctrl+V in an Explorer folder such as `C:\Temp\paste`. After about 10 s press Cancel in Explorer's copy dialog. Note the time you pressed Cancel.
- Expect: the reads stop at once. No further `stream` or read lines follow the cancel. The `stream released` line shows `completed=false` and `served` well below `size`.
- Record: the time from Cancel to the `stream released` line (or to the last console line), the `served` and `size` values, and the console output after the cancel.

### Step 11 - winvfiles: blocking Read (Explorer responsiveness)
- Do: Stop winvfiles with Ctrl+C. Run `.\bin\winvfiles.exe -blockread 3s`. Press Ctrl+V in `C:\Temp\paste`. During the 3 s stall, try to move the Explorer window, open another folder, right-click on the desktop and type into the address bar.
- Expect: the first read stalls for 3 s. Record which of these responded during the stall: the window move, the other folder, the context menu, the address bar typing. The paste completes correctly afterwards.
- Record: the responsiveness for each of the four actions (responded or frozen), the copy dialog behaviour, and whether the paste result is correct.

### Step 12 - winvfiles: exit without flush
- Do: Stop winvfiles with Ctrl+C. Run `.\bin\winvfiles.exe` (no flag). Wait for the `virtual files are on the clipboard` line. Press Ctrl+C. Then press Ctrl+V in Explorer on `C:\Temp\paste`.
- Expect: the console prints `exiting without OleFlushClipboard`. Record exactly what Explorer does on paste: nothing, an error dialog, old clipboard content or an empty paste.
- Record: the console output after Ctrl+C, the exact paste behaviour, and whether a clipboard viewer (for example the Win+V clipboard history, if enabled) still lists any formats.

### Step 13 - winvfiles: exit with flush
- Do: Run `.\bin\winvfiles.exe -flush`. Wait for the `virtual files are on the clipboard` line. Press Ctrl+C and watch the console until the program returns to the prompt. Then press Ctrl+V in Explorer on `C:\Temp\paste`.
- Expect: the console prints `OleFlushClipboard returned S_OK`. Explorer pastes the virtual files as in step 8, or reports what happened exactly. Watch the log during the flush: a flush that renders the 200 MiB stream shows many reads and takes about 12 s at the default rate.
- Record: the console output from Ctrl+C to the prompt, the number of `IDataObject call` lines during the flush and their `tymed` values, the time the flush took, and what Explorer pasted.

### Step 14 - winvfiles: ownership
- Do: Run `.\bin\winvfiles.exe`. Wait for the `virtual files are on the clipboard` line. Click into Notepad, type `other`, select it and press Ctrl+C. Note the time of the copy.
- Expect: the console logs `clipboard ownership lost: another copy replaced the virtual files` within about 1 s of the copy.
- Record: the time difference between the copy and the warning line (from the console timestamps), and the warning line itself.
- Then stop winvfiles with Ctrl+C.

### Step 15 - winvfiles: elevated Explorer (optional, UIPI)
- Do: Needs an elevated window. The README allows an elevated File Explorer instance or an elevated file manager. Use whichever your PC allows and record which one you used. Run `.\bin\winvfiles.exe` (unelevated). Paste into a folder shown by the elevated window.
- Expect: whether the paste into the elevated window works, and what the console and the Explorer window show.
- Record: the elevated window you used and the result (works, prohibited sign, error text).

### Step 16 - windnd: edge target, plain files (run W1)
- Do: Run `.\bin\windnd.exe`. Read the startup line: it shows the `edge` rectangle, `callbacks` and the Scroll Lock warning. Do not press Scroll Lock. In Explorer open `C:\Temp\dnd`, select `one.txt` and `two.txt`, drag them to the right edge of the primary monitor, hold for one second and release. Repeat once and move away without releasing.
- Expect: a `DragEnter` line with `inside_edge=true` and a `formats` list. Explorer offers `CF_HDROP` and `FileGroupDescriptorW` (the list may contain more formats). A `Drop` line with the same payload after release. A `DragLeave` line after moving away. The cursor shows the copy (plus sign) over the strip. Holding Ctrl, Shift and then Ctrl+Shift during the drag produces `DragOver state changed` lines. The returned effect stays `copy`.
- Record: the `DragEnter` and `Drop` lines (formats and paths), the `DragLeave` line, the number of `DragOver` calls the summary reports, and the cursor you saw.
- Optional, after step 18 and after stopping run W1 with Ctrl+C: run `.\bin\windnd.exe -edge 4`, repeat the drop of `one.txt` and `two.txt` onto the edge, and record yes/no for a `DragEnter` line.

### Step 17 - windnd: folders, multi-selection and a zip (run W1)
- Do: Still in run W1, drag the folder `C:\Temp\dnd\sub` to the edge. Then drag `one.txt` and `two.txt` together again. Then open `pack.zip` in Explorer, select `one.txt` inside it and drag it to the edge.
- Expect: for each drag a `DragEnter` and `Drop` pair with the paths. The zip entry arrives as a virtual file: `FileGroupDescriptorW` and `FileContents` are listed, and the path is the zip entry name.
- Record: the formats and paths of each of the three drags.

### Step 18 - windnd: click-through and DragEnter reliability (run W1)
- Do: With windnd idle, click, hold and drag in applications at the right screen edge (a maximised window's scroll bar or close button at the edge, the desktop, a snap gesture at the edge). Then press and hold the left button in a neutral place, move to the edge and release. Then repeat the Explorer drag from step 16 at least 20 times, varying speed and approach direction.
- Expect: the applications at the edge respond normally while idle. The `edge click-through changed` lines show `click_through=false` on the press outside the strip and `click_through=true` on the release. After every gesture the program prints `left button gesture finished` with `crossed_edge_while_pressed` and `dragenter_fired`. A gesture with `crossed_edge_while_pressed=true` and `dragenter_fired=false` also prints a warning line.
- Record: whether a normal press and hold at the edge behaves as before the program ran (yes/no and what you saw), the number of gestures in total, the number with `crossed_edge_while_pressed=true` and `dragenter_fired=false`, and the number with `dragenter_fired=true`.
- Open point (click-through and DragEnter reliability): this step is the reliability measurement. Record the counts exactly.
- Then stop run W1 with Ctrl+C and paste the summary lines (`edge target summary`, `hook summary`).

### Step 19 - windnd: drag source to the desktop (run W2)
- Do: Run `.\bin\windnd.exe` again. Press the left button in the window `EdgeSync windnd drag source`, keep it held, move onto the desktop and release. Only throw-away files are dropped: the result is a `demo` folder and `b.txt` on the desktop.
- Expect: `IDataObject call` lines (which `GetData` format, `lindex`, `tymed` and `aspect`, in which order), `stream created, nothing read yet` and `stream released` lines, `GiveFeedback effect changed` lines, `DoDragDrop returned` with `hresult` and `outcome`, and `paste outcome reported` (the `Performed DropEffect` `SetData`). The files appear on the desktop: `demo\a.bin` (200 MiB) and `b.txt` (10 KiB).
- Record: the `DoDragDrop returned` line, the list of `IDataObject call` lines, the `stream released` lines (with `completed`), the names, sizes and times of the desktop files, and whether Explorer showed a progress dialog.

### Step 20 - windnd: drag source to an Explorer folder (run W2)
- Do: In run W2, drag from the source window to Explorer's `C:\Temp\dnd-out` folder and release.
- Expect: the same kind of lines as step 19. The files appear in `C:\Temp\dnd-out`.
- Record: the `DoDragDrop returned` line, `stream released` lines, the files that appeared, and the drop outcome line.

### Step 21 - windnd: drag source cancel and misses (run W2)
- Do: In run W2, start a drag in the source window and press Escape while holding the button, then release. Start another drag and release over a window that accepts no drops (for example the Windows taskbar background). Start a third drag and release inside the source window itself.
- Expect: `QueryContinueDrag` with `decision=cancel` and `outcome=cancelled` after Escape. The release over the non-accepting window and inside the source window produce `DoDragDrop returned` lines with an outcome that describes the miss.
- Record: the three `DoDragDrop returned` lines and the cursor shown for each gesture.

### Step 22 - windnd: drag source to an Explorer folder with cancel (run W3)
- Do: Stop run W2 with Ctrl+C. Run `.\bin\windnd.exe -rate 4 -latency 500ms`. Drag from the source window to `C:\Temp\dnd-out`, release, and after about 10 s press Cancel in Explorer's copy dialog.
- Expect: reads stop at once after Cancel. The `stream released` line shows `completed=false`.
- Record: the time from Cancel to the last read line, and the `stream released` line.
- Then stop run W3 with Ctrl+C.

### Step 23 - windnd: drag source onto its own edge strip (run W4)
- Do: Run `.\bin\windnd.exe`. Start a drag in the source window and release on the right edge strip of the primary monitor.
- Expect: `DragEnter` and `Drop` lines with the payload `FileGroupDescriptorW` listing three entries and no `CF_HDROP`. The program does not hang.
- Record: the `DragEnter` and `Drop` lines and whether anything hung (yes/no).
- Then stop windnd with Ctrl+C and paste the complete console output from the first line to the summary lines (`edge target summary`, `hook summary`, `drag source session`, `file accounting`, `drop outcomes`).

### Step 24 - windnd: elevated UIPI matrix (optional, needs admin)
- Do: Use an elevated Explorer window or an elevated file manager as in step 15, and record which one you used. Then run four combinations:
  - a. Unelevated `.\bin\windnd.exe`; drag files from the elevated window to the right edge strip.
  - b. Unelevated `.\bin\windnd.exe`; drag from the source window onto a folder in the elevated window.
  - c. Start PowerShell with "Run as administrator", go to `spikes`, run `.\bin\windnd.exe`; drag files from an unelevated Explorer window to the strip.
  - d. The same elevated `windnd.exe` as in c; drag files from the elevated window to the strip.
- Expect: for a, a gesture summary with `crossed_edge_while_pressed=true` and `dragenter_fired=false`, and a prohibited cursor sign if Windows shows one. For b, a drop or a prohibited cursor sign. For c and d, which sources the strip receives.
- Record: the result of each combination (works, prohibited cursor, warning line), and the gesture summary lines.
- Open point (UIPI): this step settles whether elevated sources reach the unelevated strip and whether an elevated strip receives unelevated drags.

### Step 25 - giodrop: window HWND and OleInitialize (repository root)
- Do: From the repository root run `.\spikes\bin\giodrop.exe`. Read the window text.
- Expect: a 720 x 480 dp window titled `EdgeSync Gio file-drop spike`. First the text `OLE drop target: not registered`. Then `Window HWND: 0x...` (non-zero), `OleInitialize on the window thread: S_OK (first COM initialisation on this thread)` and `OLE drop target: registered on the Gio window thread`.
- Record: the full window text after start, including the HWND value.
- Open point (OleInitialize on the Gio thread): `S_OK` means no other component had initialised COM on that thread. `S_FALSE` means something else did. Record the value shown exactly. Also record whether the program printed anything on stderr (expected: nothing).

### Step 26 - giodrop: drop files from Explorer
- Do: Select two or more files in `C:\Temp\dnd` and drag them onto the Gio window. Release the button over the window. Then drop once more with two files.
- Expect: the cursor shows the copy badge over the window. After the first drop `Drops: 1`, then `Drops: 2` after the second. A `Last drop point` line in dp and in client px, and one line per dropped path with the full path (`C:\...`, no trailing separator).
- Record: the window text after each drop (the drop point line and the path lines).

### Step 27 - giodrop: drop text (error case)
- Do: Select a word in Notepad and drag the selection onto the Gio window. Release.
- Expect: a `Last error: drop data: ...` line naming the `CF_HDROP` failure. `Drops` does not change.
- Record: the exact `Last error` line.

### Step 28 - giodrop: DPI and scale
- Do: Drop one file at a fixed point in the window and record the `Last drop point` dp value. Then either move the window to a monitor with another scale or change the display scale (Settings > System > Display > Scale). Drop a file at the same physical spot again. If only one scale is available on your PC, say so.
- Expect: the same physical spot gives the same dp value on both scales.
- Record: the two dp values, the scales of the monitors (or the two scale settings) and yes/no for "the same dp value".

### Step 29 - giodrop: close
- Do: Close the window with its title-bar button.
- Expect: the process ends. Nothing is printed on stderr. Run `Get-Process giodrop -ErrorAction SilentlyContinue` and expect no output. If your PowerShell prompt only returned after the window closed, run `$LASTEXITCODE` and expect `0`.
- Record: the output of the two checks and yes/no for "nothing on stderr".

## Cleanup
1. Stop winclip, winvfiles and windnd: press Ctrl+C in each console and wait until the prompt returns. winvfiles prints `file accounting` and `paste outcomes`. windnd prints `edge target summary`, `hook summary`, `drag source session`, `file accounting` and `drop outcomes`. Paste those lines if you have not yet.
2. Close giodrop with its title-bar button.
3. Confirm no spike process is left: `Get-Process winclip, winvfiles, windnd, giodrop -ErrorAction SilentlyContinue`. Expect no output.
4. Restore the clipboard: copy a short text of your own in Notepad. This replaces any offer the spikes left. If clipboard history is on, clear it in Settings > System > Clipboard > Clear clipboard data.
5. Remove the test output: delete the contents of `C:\Temp\paste` and `C:\Temp\dnd-out` (they contain the 200 MiB file), and delete `demo` and `b.txt` from the Desktop folder that step 19 created. Remove the scratch folders `C:\Temp\dnd` and `C:\Temp\dnd-out` if you no longer need them.
6. Record the results in `qa/manual/results/p0-sp2-windows.md`, using the Results format in `qa/manual/README.md`: the date, the tester, the commit (`git rev-parse --short HEAD` in the repository root), the machine (Windows version and build from step 2), one row per step (mark optional steps that were skipped with "not run" and the reason), the verdict and the notes. Paste the full text of that file in chat, or the parent commits it.

## Safety
- windnd installs the low-level mouse and keyboard hooks. Scroll Lock toggles input suppression: the first press blocks all keyboard and mouse input system-wide (Scroll Lock itself still works), and the next press lets input through again. Do not press Scroll Lock during windnd runs. If input stops working, press Scroll Lock once more. If that does not help, press Ctrl+Alt+Delete, open Task Manager and end `windnd.exe`, or close its console window.
- Keep the PC free of important focus. winclip's Ctrl+V pastes into whatever window has focus. The drag tests drop files on folders and on the desktop, and the winvfiles paste writes 200 MiB. Save and close other work first.
- The windnd edge strip is an invisible topmost 2 px window on the right edge of the primary monitor. If the pointer seems stuck near that edge, close windnd.
- Use throw-away files only: `C:\Temp\dnd` and the folders named in this script.
- Stop any program with Ctrl+C in its console. If a console does not respond, close its window, which ends the process.
- The optional elevated steps (15, 24) need an elevated window or prompt that you open yourself. Close them when done. Do not run any installer. The spikes do not install a service.
- Only the binaries in `spikes\bin` are started. Nothing else needs to be installed or changed.
