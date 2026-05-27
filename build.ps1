param(
    [Parameter(Mandatory=$false)]
    [string]$Version
)

if ($Version) {
    $tag = "v$Version"
    Write-Host "Membuat dan mem-push tag $tag..." -ForegroundColor Green
    git tag $tag
    git push origin $tag
} else {
    Write-Host "Tidak ada parameter -Version yang diberikan, melewati proses git tag." -ForegroundColor Yellow
}

$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -ldflags "-s -w" -o if-slr.exe .\cmd\app\
Compress-Archive -Path .\if-slr.exe, .\.env -DestinationPath .\aplikasi-slr.zip -Force