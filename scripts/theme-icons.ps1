# Renders the theme icons in frontend/public/themes to the PNGs the app sets on
# its window (app/icons), and builds the exe icon resource from the default
# theme. Run it after changing an icon SVG; needs Microsoft Edge and wails3.
$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $true
$root = Split-Path $PSScriptRoot -Parent
$edge = @("${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe", "$env:ProgramFiles\Microsoft\Edge\Application\msedge.exe") |
  Where-Object { Test-Path $_ } | Select-Object -First 1
if (-not $edge) { throw "Microsoft Edge not found" }

$out = Join-Path $root "app\icons"
New-Item -ItemType Directory -Force $out | Out-Null
$profile = Join-Path $env:TEMP "go-fish-edge" # its own profile, so a running Edge does not take the call
$page = Join-Path $env:TEMP "go-fish-icon.html"
foreach ($svg in Get-ChildItem (Join-Path $root "frontend\public\themes\*.svg")) {
  # Headless Edge keeps a wider viewport than asked, so the icon is pinned top left.
  $src = ([Uri]$svg.FullName).AbsoluteUri
  Set-Content $page "<html><body style='margin:0;background:transparent'><img src='$src' width='256' height='256' style='position:fixed;left:0;top:0'></body></html>"
  $png = Join-Path $out ($svg.BaseName + ".png")
  Remove-Item $png -ErrorAction SilentlyContinue
  Start-Process $edge -Wait -ArgumentList "--headless=new", "--disable-gpu", "--hide-scrollbars", "--user-data-dir=$profile",
    "--default-background-color=00000000", "--window-size=256,256", "--screenshot=$png", ([Uri]$page).AbsoluteUri
  if (-not (Test-Path $png)) { throw "rendering $($svg.Name) failed" }
  Write-Host "rendered $png"
}
Remove-Item $page

# The exe carries the default theme's icon; go build links the .syso next to main.go.
$build = Join-Path $root "build\windows"
New-Item -ItemType Directory -Force $build | Out-Null
wails3 generate icons -input (Join-Path $out "graphite.png") -windowsfilename (Join-Path $build "icon.ico") -macfilename (Join-Path $env:TEMP "go-fish.icns")
wails3 generate syso -arch amd64 -icon (Join-Path $build "icon.ico") -manifest (Join-Path $build "wails.exe.manifest") -out (Join-Path $root "rsrc_windows_amd64.syso")
