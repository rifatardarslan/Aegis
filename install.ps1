# Build locally or install a checksum-verified release. No system security policy is changed.
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
$aegisTemp = Join-Path ([IO.Path]::GetTempPath()) ('aegis-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $aegisTemp | Out-Null
try {
    $aegisBinary = Join-Path $aegisTemp 'aegis.exe'
    $aegisBuilt = $false
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        if (Get-Command winget -ErrorAction SilentlyContinue) {
            & winget install GoLang.Go --silent --accept-source-agreements --accept-package-agreements
            $aegisGoPath = Join-Path $env:ProgramFiles 'Go\bin'
            if (Test-Path (Join-Path $aegisGoPath 'go.exe')) { $env:PATH += ';' + $aegisGoPath }
        }
    }
    if (Get-Command go -ErrorAction SilentlyContinue) {
        & go build -trimpath -o $aegisBinary ./cmd/aegis
        $aegisBuilt = $LASTEXITCODE -eq 0
    }
    if (-not $aegisBuilt) {
        $aegisArch = [Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
        if ($aegisArch -eq 'x64') { $aegisArch = 'amd64' }
        if ($aegisArch -notin @('amd64', 'arm64')) { throw 'Unsupported architecture.' }
        $aegisAsset = "aegis-windows-$aegisArch.exe"
        $aegisRelease = 'https://github.com/rifatardarslan/Aegis/releases/latest/download'
        $aegisSums = Join-Path $aegisTemp 'SHA256SUMS'
        Invoke-WebRequest -UseBasicParsing -Uri "$aegisRelease/$aegisAsset" -OutFile $aegisBinary
        Invoke-WebRequest -UseBasicParsing -Uri "$aegisRelease/SHA256SUMS" -OutFile $aegisSums
        $aegisMatches = @(Get-Content $aegisSums | Where-Object { $_ -match ('^[0-9a-fA-F]{64}\s+\*?' + [Regex]::Escape($aegisAsset) + '$') })
        if ($aegisMatches.Count -ne 1) { throw 'Missing or invalid release checksum.' }
        $aegisExpected = ($aegisMatches[0] -split '\s+')[0]
        $aegisActual = (Get-FileHash -Algorithm SHA256 $aegisBinary).Hash
        if ($aegisActual -ne $aegisExpected) { throw 'Release checksum mismatch; nothing installed.' }
    }
    $aegisInstall = Join-Path $env:USERPROFILE '.aegis\bin'
    New-Item -ItemType Directory -Force -Path $aegisInstall | Out-Null
    Copy-Item $aegisBinary (Join-Path $aegisInstall 'aegis.exe') -Force
    $aegisUserPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
    if (($aegisUserPath -split ';') -notcontains $aegisInstall) {
        [Environment]::SetEnvironmentVariable('PATH', ($aegisUserPath + ';' + $aegisInstall), 'User')
    }
    Write-Host 'Aegis installed. Open a new terminal and run aegis.'
} finally {
    Remove-Item -LiteralPath $aegisTemp -Recurse -Force -ErrorAction SilentlyContinue
}
