Set-Location 'C:\Users\Pranav bhardwaj\.gemini\antigravity\scratch\week1-core-proxy'
$proc = Start-Process -FilePath '.\core-proxy.exe' -ArgumentList '-port 9876 -static ./public' -PassThru
Start-Sleep -Milliseconds 800

try {
    $health = Invoke-RestMethod -Uri 'http://localhost:9876/api/health'
    Write-Output "=== HEALTH ENDPOINT RESPONSE ==="
    $health | ConvertTo-Json

    $html = (Invoke-WebRequest -Uri 'http://localhost:9876/' -UseBasicParsing).Content
    Write-Output "=== STATIC HTML SERVED (LENGTH: $($html.Length)) ==="
    Write-Output $html.Substring(0, 150)
} finally {
    if ($proc -and !$proc.HasExited) {
        Stop-Process -Id $proc.Id -Force
    }
}
