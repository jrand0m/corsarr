# Run corsarr at logon as the current user, restarted on failure, with no admin rights.
#   powershell -ExecutionPolicy Bypass -File corsarr-task.ps1
# Remove: Unregister-ScheduledTask -TaskName corsarr -Confirm:$false
$ErrorActionPreference = 'Stop'

$exe = (Get-Command corsarr -ErrorAction Stop).Source
$data = Join-Path $env:LOCALAPPDATA 'corsarr'
New-Item -ItemType Directory -Force -Path $data | Out-Null

$action = New-ScheduledTaskAction -Execute $exe -Argument "--data.dir=`"$data`"" -WorkingDirectory $data
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$settings = New-ScheduledTaskSettingsSet -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) `
  -ExecutionTimeLimit ([TimeSpan]::Zero) -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
Register-ScheduledTask -TaskName 'corsarr' -Action $action -Trigger $trigger -Settings $settings `
  -Description 'corsarr media automation server' -Force | Out-Null
Start-ScheduledTask -TaskName 'corsarr'
Write-Host "corsarr runs at logon. Data: $data  Web UI: http://localhost:8090"
