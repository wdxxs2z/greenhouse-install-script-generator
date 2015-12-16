$scriptpath = $MyInvocation.MyCommand.Path
$dir = Split-Path $scriptpath
$versioninfoPath = "..\src\generate\versioninfo.json"
push-location $dir
   $versioninfo = (get-content $versioninfoPath) | ConvertFrom-Json
   $versioninfo.StringFileInfo.ProductVersion = "$env:APPVEYOR_BUILD_VERSION-$env:APPVEYOR_REPO_COMMIT"
   ConvertTo-Json $versioninfo | Set-Content $versioninfoPath 
pop-location