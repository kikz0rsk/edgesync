<#
.SYNOPSIS
    Installs and starts the TEMPORARY EdgeSyncSpike service (P0-SP1-004 spike). Run from an elevated PowerShell.

.DESCRIPTION
    - Copies spikesvc.exe and spikehelper.exe from -SourceDir into %ProgramData%\EdgeSyncSpike\bin\.
    - Locks %ProgramData%\EdgeSyncSpike\ so only SYSTEM and Administrators can write there (the helper runs as SYSTEM).
    - Creates (or updates) the service EdgeSyncSpike running as LocalSystem, start type Manual (or Automatic with -AutoStart).
    - The service stops itself after -MaxRunMinutes (default 30).
    - Writes ALL logs to %ProgramData%\EdgeSyncSpike\ (service.log, helper.log).
    - Only with -EnableSasPolicy: sets the machine policy SoftwareSASGeneration so that SendSAS works for services. The previous
      value is saved to %ProgramData%\EdgeSyncSpike\sas-policy-backup.json BEFORE the change and uninstall.ps1 restores it.
    Running this script twice is safe: the service is updated in place and the policy backup keeps the ORIGINAL value.

.PARAMETER SourceDir
    Folder with spikesvc.exe and spikehelper.exe. Default: the spikes\bin folder next to this script.

.PARAMETER MaxRunMinutes
    Hard self-stop of the service, 1 to 240 minutes. Default 30.

.PARAMETER Inject
    The helper injects a 200 px pointer square, the text "edgesync" and Escape on the current input desktop every 10 seconds.
    Keep the machine free of important focus while it runs: the text and Escape go to whatever window has focus.

.PARAMETER EnableSasPolicy
    OPT-IN. Changes HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System\SoftwareSASGeneration (see the README) and makes
    the helper ask the service for ONE Ctrl+Alt+Del screen about 15 seconds after it starts (at most 3 per service run).

.PARAMETER AutoStart
    Start type Automatic instead of Manual, needed for the "reboot with nobody logged on" test. The service still stops itself
    after -MaxRunMinutes of every boot until you run uninstall.ps1.

.PARAMETER LaunchDesktop
    lpDesktop used when the service creates the helper: winlogon (default) or default.

.PARAMETER NoStart
    Install without starting the service.
#>
[CmdletBinding()]
param(
    [string]$SourceDir = (Join-Path $PSScriptRoot '..\bin'),
    [ValidateRange(1, 240)][int]$MaxRunMinutes = 30,
    [switch]$Inject,
    [switch]$EnableSasPolicy,
    [switch]$AutoStart,
    [ValidateSet('winlogon', 'default')][string]$LaunchDesktop = 'winlogon',
    [switch]$NoStart
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
$AllowedOwners = @('S-1-5-18', 'S-1-5-32-544', 'S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464')

function Write-Step([string]$Message) {
    Write-Host ('[ACTION] ' + $Message)
}

function Stop-Install([string]$Message) {
    Write-Host ('[ERROR] ' + $Message) -ForegroundColor Red
    exit 1
}

function Get-SasPolicy {
    $properties = Get-ItemProperty -Path $PolicyKey
    if ($properties.PSObject.Properties.Name -contains $PolicyName) {
        return [int]$properties.$PolicyName
    }
    return $null
}

function Test-OurService($Service) {
    return ($null -ne $Service.PathName) -and ($Service.PathName -like ('*' + $SvcExe + '*'))
}

$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Stop-Install 'This script must run in an ELEVATED PowerShell (right-click PowerShell, Run as administrator). Nothing was changed.'
}
if (-not [Environment]::Is64BitProcess) {
    Stop-Install 'Run this script in 64-bit PowerShell (not the x86 shortcut). Nothing was changed.'
}

$SourceDir = (Resolve-Path -Path $SourceDir).Path
$srcSvc = Join-Path $SourceDir 'spikesvc.exe'
$srcHelper = Join-Path $SourceDir 'spikehelper.exe'
foreach ($file in @($srcSvc, $srcHelper)) {
    if (-not (Test-Path -Path $file -PathType Leaf)) {
        Stop-Install ('Missing ' + $file + '. Build it first: see README.md. Nothing was changed.')
    }
}

Write-Host ''
Write-Host 'EdgeSyncSpike INSTALL (temporary spike, removable with uninstall.ps1)'
Write-Host ('  data and logs : ' + $DataDir)
Write-Host ('  service       : ' + $ServiceName + ' (LocalSystem), self-stop after ' + $MaxRunMinutes + ' minutes')
Write-Host ('  start type    : ' + $(if ($AutoStart) { 'Automatic' } else { 'Manual' }))
Write-Host ('  helper inject : ' + $Inject.IsPresent)
Write-Host ('  SAS policy    : ' + $(if ($EnableSasPolicy) { 'WILL BE CHANGED (opt-in)' } else { 'untouched' }))
Write-Host ''

$existing = Get-CimInstance -ClassName Win32_Service -Filter ("Name='" + $ServiceName + "'")
if ($null -ne $existing -and -not (Test-OurService $existing)) {
    Stop-Install ('A service named ' + $ServiceName + ' already exists with another binary (' + $existing.PathName + '). Refusing to touch it.')
}

if (Test-Path -Path $DataDir) {
    $ownerSid = (Get-Acl -Path $DataDir).GetOwner([System.Security.Principal.SecurityIdentifier]).Value
    if ($AllowedOwners -notcontains $ownerSid) {
        Stop-Install ($DataDir + ' exists and is owned by ' + $ownerSid + ', not by SYSTEM or Administrators. Inspect and delete it yourself, then run again.')
    }
} else {
    Write-Step ('Creating directory ' + $DataDir)
    New-Item -ItemType Directory -Path $DataDir | Out-Null
}

Write-Step ('Restricting ' + $DataDir + ' to SYSTEM and Administrators (full control, inheritance from ProgramData removed)')
$acl = New-Object System.Security.AccessControl.DirectorySecurity
$acl.SetAccessRuleProtection($true, $false)
$inheritFlags = [System.Security.AccessControl.InheritanceFlags]'ContainerInherit, ObjectInherit'
$noPropagation = [System.Security.AccessControl.PropagationFlags]::None
foreach ($sid in @('S-1-5-18', 'S-1-5-32-544')) {
    $identity = New-Object System.Security.Principal.SecurityIdentifier($sid)
    $rule = New-Object System.Security.AccessControl.FileSystemAccessRule($identity, 'FullControl', $inheritFlags, $noPropagation, 'Allow')
    $acl.AddAccessRule($rule)
}
Set-Acl -Path $DataDir -AclObject $acl

if (-not (Test-Path -Path $BinDir)) {
    Write-Step ('Creating directory ' + $BinDir)
    New-Item -ItemType Directory -Path $BinDir | Out-Null
}

if ($null -ne $existing) {
    $running = Get-Service -Name $ServiceName
    if ($running.Status -ne 'Stopped') {
        Write-Step ('Stopping the running service ' + $ServiceName + ' before replacing its binaries')
        Stop-Service -Name $ServiceName -Force
        $running.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(30))
    }
}

Write-Step ('Copying ' + $srcSvc + ' to ' + $SvcExe)
Copy-Item -Path $srcSvc -Destination $SvcExe -Force
Write-Step ('Copying ' + $srcHelper + ' to ' + $HelperExe)
Copy-Item -Path $srcHelper -Destination $HelperExe -Force

if ($EnableSasPolicy) {
    $current = Get-SasPolicy
    $currentText = if ($null -eq $current) { 'not set' } else { [string]$current }
    Write-Host ('SoftwareSASGeneration currently: ' + $currentText)
    if (Test-Path -Path $BackupFile) {
        Write-Host ('A policy backup already exists (' + $BackupFile + '); it is kept so uninstall restores the ORIGINAL value.')
    } else {
        if ($null -ne $current -and ($current -eq 1 -or $current -eq 3)) {
            $newValue = $current
        } elseif ($null -ne $current -and $current -eq 2) {
            $newValue = 3
        } else {
            $newValue = 1
        }
        $backup = [ordered]@{
            existed  = ($null -ne $current)
            previous = $current
            set      = $newValue
            changed  = ($current -ne $newValue)
        }
        Write-Step ('Saving the previous policy value to ' + $BackupFile)
        ($backup | ConvertTo-Json) | Set-Content -Path $BackupFile -Encoding ASCII
        if ($backup.changed) {
            Write-Step ('Setting ' + $PolicyKey + '\' + $PolicyName + ' = ' + $newValue + ' (DWord)')
            Set-ItemProperty -Path $PolicyKey -Name $PolicyName -Value $newValue -Type DWord
        } else {
            Write-Host 'The policy already allows services; nothing to change.'
        }
    }
}

$serviceArgs = '-maxrun {0}m -dir "{1}" -desktop {2}' -f $MaxRunMinutes, $DataDir, $LaunchDesktop
if ($Inject) { $serviceArgs += ' -helper-inject' }
if ($EnableSasPolicy) { $serviceArgs += ' -allow-sas' }
$binaryPath = '"{0}" {1}' -f $SvcExe, $serviceArgs
$startMode = if ($AutoStart) { 'Automatic' } else { 'Manual' }
$description = 'TEMPORARY EdgeSync spike (P0-SP1-004). Remove with uninstall.ps1. Stops itself after ' + $MaxRunMinutes + ' minutes.'

if ($null -eq $existing) {
    Write-Step ('Creating service ' + $ServiceName + ' : ' + $binaryPath)
    New-Service -Name $ServiceName -BinaryPathName $binaryPath -DisplayName 'EdgeSync Spike (temporary)' -Description $description -StartupType $startMode | Out-Null
} else {
    Write-Step ('Updating service ' + $ServiceName + ' : ' + $binaryPath)
    $result = Invoke-CimMethod -InputObject $existing -MethodName Change -Arguments @{ PathName = $binaryPath; StartMode = $startMode }
    if ($result.ReturnValue -ne 0) {
        Stop-Install ('Updating the service failed, Win32_Service.Change returned ' + $result.ReturnValue)
    }
}

if ($NoStart) {
    Write-Host 'Installed, not started (-NoStart). Start it with: Start-Service EdgeSyncSpike'
} else {
    Write-Step ('Starting service ' + $ServiceName)
    Start-Service -Name $ServiceName
    Get-Service -Name $ServiceName | Format-Table -AutoSize Name, Status, StartType
}

Write-Host ''
Write-Host ('Logs: ' + (Join-Path $DataDir 'service.log') + ' and ' + (Join-Path $DataDir 'helper.log'))
Write-Host ('The service stops itself after ' + $MaxRunMinutes + ' minutes.')
Write-Host 'Stop now   : Stop-Service EdgeSyncSpike -Force   (or: sc.exe stop EdgeSyncSpike)'
Write-Host 'Remove all : run uninstall.ps1 in an elevated PowerShell (-PurgeLogs also deletes the logs).'
exit 0
