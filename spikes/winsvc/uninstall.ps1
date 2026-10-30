<#
.SYNOPSIS
    Removes the TEMPORARY EdgeSyncSpike service and everything install.ps1 changed. Run from an elevated PowerShell.

.DESCRIPTION
    - Stops the service EdgeSyncSpike (killing its process if it does not stop) and deletes it. Absent service is not an error.
    - Kills any spikehelper.exe running from %ProgramData%\EdgeSyncSpike\bin\.
    - Restores SoftwareSASGeneration from %ProgramData%\EdgeSyncSpike\sas-policy-backup.json: the previous value is written back,
      or the value is deleted when there was none. If the value no longer equals what install.ps1 set (someone or a Group Policy
      changed it since) it is left alone and the backup file is kept; the script says so.
    - Keeps the directory %ProgramData%\EdgeSyncSpike\ (binaries and logs) unless -PurgeLogs is given.
    Running this script twice, or on a machine where nothing is installed, is safe.

.PARAMETER PurgeLogs
    Also delete %ProgramData%\EdgeSyncSpike\ completely. Refused while an unresolved policy backup remains.
#>
[CmdletBinding()]
param(
    [switch]$PurgeLogs
)

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

$ServiceName = 'EdgeSyncSpike'
$PolicyKey = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System'
$PolicyName = 'SoftwareSASGeneration'
$DataDir = Join-Path $env:ProgramData 'EdgeSyncSpike'
$BinDir = Join-Path $DataDir 'bin'
$BackupFile = Join-Path $DataDir 'sas-policy-backup.json'
$SvcExe = Join-Path $BinDir 'spikesvc.exe'
$HelperExe = Join-Path $BinDir 'spikehelper.exe'

function Write-Step([string]$Message) {
    Write-Host ('[ACTION] ' + $Message)
}

function Get-SasPolicy {
    $properties = Get-ItemProperty -Path $PolicyKey
    if ($properties.PSObject.Properties.Name -contains $PolicyName) {
        return [int]$properties.$PolicyName
    }
    return $null
}

$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Host '[ERROR] This script must run in an ELEVATED PowerShell (right-click PowerShell, Run as administrator). Nothing was changed.' -ForegroundColor Red
    exit 1
}
if (-not [Environment]::Is64BitProcess) {
    Write-Host '[ERROR] Run this script in 64-bit PowerShell (not the x86 shortcut). Nothing was changed.' -ForegroundColor Red
    exit 1
}

Write-Host ''
Write-Host 'EdgeSyncSpike UNINSTALL'
Write-Host ('  data and logs : ' + $DataDir + $(if ($PurgeLogs) { '  (WILL BE DELETED, -PurgeLogs)' } else { '  (kept)' }))
Write-Host ''

$service = Get-CimInstance -ClassName Win32_Service -Filter ("Name='" + $ServiceName + "'")
if ($null -eq $service) {
    Write-Host ('Service ' + $ServiceName + ' is not installed; nothing to stop or delete.')
} else {
    if (($null -eq $service.PathName) -or ($service.PathName -notlike ('*' + $SvcExe + '*'))) {
        Write-Host ('[ERROR] A service named ' + $ServiceName + ' exists with another binary (' + $service.PathName + '). Refusing to touch it.') -ForegroundColor Red
        exit 1
    }
    $live = Get-Service -Name $ServiceName
    if ($live.Status -ne 'Stopped') {
        Write-Step ('Stopping service ' + $ServiceName)
        try {
            Stop-Service -Name $ServiceName -Force
            $live.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(30))
        } catch {
            Write-Host ('[WARN] The service did not stop cleanly: ' + $_.Exception.Message)
            $processId = (Get-CimInstance -ClassName Win32_Service -Filter ("Name='" + $ServiceName + "'")).ProcessId
            if ($processId -ne 0) {
                Write-Step ('Killing the service process ' + $processId)
                Stop-Process -Id $processId -Force
            }
        }
    }
    Write-Step ('Deleting service ' + $ServiceName + ' (sc.exe delete)')
    & sc.exe delete $ServiceName | Out-Host
    if ($LASTEXITCODE -ne 0) {
        Write-Host ('[ERROR] sc.exe delete failed with exit code ' + $LASTEXITCODE + '. Close services.msc and Task Manager, then run this script again.') -ForegroundColor Red
        exit 1
    }
    $deadline = (Get-Date).AddSeconds(15)
    while (($null -ne (Get-CimInstance -ClassName Win32_Service -Filter ("Name='" + $ServiceName + "'"))) -and ((Get-Date) -lt $deadline)) {
        Start-Sleep -Milliseconds 500
    }
    if ($null -ne (Get-CimInstance -ClassName Win32_Service -Filter ("Name='" + $ServiceName + "'"))) {
        Write-Host '[WARN] The service is marked for deletion and disappears when its last handle closes (or after a reboot).'
    }
}

$helpers = @(Get-CimInstance -ClassName Win32_Process -Filter "Name='spikehelper.exe'" | Where-Object { $_.ExecutablePath -eq $HelperExe })
if ($helpers.Count -eq 0) {
    Write-Host 'No spikehelper.exe from the install directory is running.'
}
foreach ($helper in $helpers) {
    Write-Step ('Killing helper process ' + $helper.ProcessId)
    Stop-Process -Id $helper.ProcessId -Force
}

$unresolved = $false
if (-not (Test-Path -Path $BackupFile)) {
    Write-Host 'No SoftwareSASGeneration backup exists; the policy was never changed by install.ps1.'
} else {
    $backup = Get-Content -Path $BackupFile -Raw | ConvertFrom-Json
    if (-not $backup.changed) {
        Write-Host 'install.ps1 did not change SoftwareSASGeneration; nothing to restore.'
        Write-Step ('Removing ' + $BackupFile)
        Remove-Item -Path $BackupFile -Force
    } else {
        $current = Get-SasPolicy
        $alreadyRestored = ($null -eq $current) -and (-not $backup.existed)
        if ($alreadyRestored) {
            Write-Host 'SoftwareSASGeneration is already absent, as before the install.'
        } elseif (($null -ne $current) -and ($current -eq [int]$backup.set)) {
            if ($backup.existed) {
                Write-Step ('Restoring ' + $PolicyKey + '\' + $PolicyName + ' = ' + $backup.previous)
                Set-ItemProperty -Path $PolicyKey -Name $PolicyName -Value ([int]$backup.previous) -Type DWord
            } else {
                Write-Step ('Deleting ' + $PolicyKey + '\' + $PolicyName + ' (it did not exist before the install)')
                Remove-ItemProperty -Path $PolicyKey -Name $PolicyName
            }
        } else {
            $currentText = if ($null -eq $current) { 'not set' } else { [string]$current }
            Write-Host ('[WARN] SoftwareSASGeneration is now ' + $currentText + ' but install.ps1 set ' + $backup.set + '. Someone else changed it, so it is NOT touched.')
            Write-Host ('       The backup stays in ' + $BackupFile + ' (existed=' + $backup.existed + ', previous=' + $backup.previous + '). Restore it by hand if you want.')
            $unresolved = $true
        }
        if (-not $unresolved) {
            Write-Step ('Removing ' + $BackupFile)
            Remove-Item -Path $BackupFile -Force
        }
    }
}

if ($PurgeLogs) {
    if ($unresolved) {
        Write-Host ('[WARN] -PurgeLogs skipped: the unresolved policy backup in ' + $DataDir + ' must not be deleted.')
    } elseif (Test-Path -Path $DataDir) {
        Write-Step ('Deleting ' + $DataDir)
        Remove-Item -Path $DataDir -Recurse -Force
    } else {
        Write-Host ($DataDir + ' does not exist; nothing to purge.')
    }
} elseif (Test-Path -Path $DataDir) {
    Write-Host ('Kept ' + $DataDir + ' (binaries and logs). Use -PurgeLogs to delete it.')
}

Write-Host ''
Write-Host 'Uninstall finished.'
exit 0
