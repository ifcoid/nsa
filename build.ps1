$env:GOOS="windows"
$env:GOARCH="amd64"
go build -ldflags "-s -w" -o if-slr.exe .\cmd\app\
Compress-Archive -Path .\if-slr.exe, .\.env -DestinationPath .\aplikasi-slr.zip