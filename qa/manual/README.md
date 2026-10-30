# Manual verification scripts

Scripts here are run by the user on real machines (D-22: agents never launch EdgeSync or spike binaries).
Results are recorded in `qa/manual/results/<same file name>.md` (the parent may paste results the user gives in
chat). A USER item in a phase file is `[x]` only when every step has a recorded observation and the verdict is pass,
or the deviation has been turned into a decision or a new item.

## File naming
`<phase>-<stream>-<topic>.md`, for example `p0-sp1-windows.md`, `p2-w1-capture.md`, `p4-f1-virtual-files.md`.

## Script format (every script uses exactly these sections)

```
# <Title>

Item: <USER item id>   Machines: <which of: Windows PC 1, Windows PC 2, GNOME box, CI (push + dispatch)>
Estimated time: <minutes>   Needs admin: yes|no

## Preparation
1. <exact command or action>

## Steps
### Step 1 - <name>
- Do: <exact commands and actions>
- Expect: <observable result>
- Record: <what to paste back: output lines, numbers, yes/no observations, screenshot names>

### Step 2 - ...

## Cleanup
1. <how to stop/uninstall everything the script started or installed>

## Safety
- <how to regain control if input gets stuck: e.g. press Scroll Lock, Ctrl+Alt+Pause, or close the console window; how to uninstall a service>
```

## Results format (`qa/manual/results/<name>.md`)

```
# Results - <script name>

Date: <YYYY-MM-DD>   Tester: user   Commit: <git short hash>
Machines: <OS name, version, build; for Linux also desktop, compositor and xdg-desktop-portal versions>

| Step | Observation | Pass |
|---|---|---|
| 1 | <text or pasted output> | yes/no |

Verdict: pass | fail
Notes: <anything unexpected>
```
