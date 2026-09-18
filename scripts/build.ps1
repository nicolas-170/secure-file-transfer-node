# Compila los dos binarios que necesita el proyecto:
#   bin\sftnode.exe  cliente para Windows (el que se ejecuta aquí)
#   bin\sftnode      binario Linux que viaja a la sede para restaurar
#
# Uso:  .\scripts\build.ps1

$ErrorActionPreference = "Stop"
Set-Location (Split-Path $PSScriptRoot -Parent)

Write-Host "Compilando cliente Windows..." -ForegroundColor Cyan
go build -o bin/sftnode.exe ./cmd/sftnode

Write-Host "Compilando binario Linux..." -ForegroundColor Cyan
$env:GOOS = "linux"; $env:GOARCH = "amd64"
go build -o bin/sftnode ./cmd/sftnode
$env:GOOS = ""; $env:GOARCH = ""

Write-Host "Listo:" -ForegroundColor Green
Get-ChildItem bin | Format-Table Name, Length -AutoSize
