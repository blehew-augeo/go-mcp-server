param(
    [string]$GitHubRepo = "",  
    [string]$BinaryName = "mcp-server.exe"
)

$InstallDir = "$env:LOCALAPPDATA\Programs\mcp-server"
$BinaryPath = Join-Path $InstallDir $BinaryName

Write-Host "MCP Server Installer" -ForegroundColor Green
Write-Host "===================="
Write-Host "Installing to: $InstallDir"
Write-Host ""

# Auto-detect repo if not specified
if ([string]::IsNullOrEmpty($GitHubRepo)) {
    # Try to detect from git remote
    try {
        $remoteUrl = & git config --get remote.origin.url 2>$null
        if ($remoteUrl) {
            # Extract owner/repo from various URL formats
            if ($remoteUrl -match "github.com[:/]([^/]+)/([^/\.]+)") {
                $owner = $Matches[1]
                $repo = $Matches[2] -replace '\.git$', ''
                $GitHubRepo = "$owner/$repo"
                Write-Host "Detected repository: $GitHubRepo" -ForegroundColor Cyan
            }
        }
    } catch {
        # Silently fail if git detection doesn't work
    }
    
    # If still empty, prompt the user
    if ([string]::IsNullOrEmpty($GitHubRepo)) {
        $GitHubRepo = Read-Host "Could not auto-detect repository. Please enter the GitHub repository (e.g., 'owner/repo')"
        if ([string]::IsNullOrEmpty($GitHubRepo)) {
            Write-Error "Repository information is required. Please try again with -GitHubRepo parameter."
            exit 1
        }
    }
}

$DownloadUrl = "https://github.com/$GitHubRepo/releases/latest/download/$BinaryName"
Write-Host "Download URL: $DownloadUrl"
Write-Host ""

try {
    if (!(Test-Path $InstallDir)) {
        Write-Host "Creating install directory..." -ForegroundColor Yellow
        New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    }
} catch {
    Write-Error "Failed to create install directory: $_"
    exit 1
}

try {
    Write-Host "Downloading latest release..." -ForegroundColor Yellow
    $ProgressPreference = 'SilentlyContinue'  
    Invoke-WebRequest -Uri $DownloadUrl -OutFile $BinaryPath
    $ProgressPreference = 'Continue'
    Write-Host "Download completed!" -ForegroundColor Green
} catch {
    Write-Error "Failed to download binary: $_"
    Write-Host "Please check that the release exists at: $DownloadUrl" -ForegroundColor Red
    exit 1
}

if (!(Test-Path $BinaryPath)) {
    Write-Error "Binary not found after download: $BinaryPath"
    exit 1
}

$CurrentPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($CurrentPath -notlike "*$InstallDir*") {
    try {
        Write-Host "Adding to PATH..." -ForegroundColor Yellow
        $NewPath = "$CurrentPath;$InstallDir"
        [Environment]::SetEnvironmentVariable("PATH", $NewPath, "User")
        Write-Host "Added to PATH successfully!" -ForegroundColor Green
        Write-Host "You may need to restart your terminal for PATH changes to take effect." -ForegroundColor Yellow
    } catch {
        Write-Warning "Failed to add to PATH: $_"
        Write-Host "You can manually add this directory to your PATH: $InstallDir" -ForegroundColor Yellow
    }
} else {
    Write-Host "Directory already in PATH" -ForegroundColor Green
}

Write-Host ""
Write-Host "Installation Summary:" -ForegroundColor Green
Write-Host "- Binary installed to: $BinaryPath"
Write-Host "- Repository: $GitHubRepo"