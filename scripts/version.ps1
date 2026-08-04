# version.ps1 - Versioning & Tag Release Helper
param (
    [string]$Type = "patch", # patch, minor, major, or exact version (e.g. v1.0.1)
    [string]$Message = ""
)

# 1. Check for uncommitted changes
$status = git status --porcelain
if ($status) {
    Write-Host "Working tree has uncommitted changes. Please commit or stash them first." -ForegroundColor Yellow
    git status -s
    exit 1
}

# 2. Get latest tag
$latestTag = git describe --tags --abbrev=0 2>$null
if (-not $latestTag) {
    $latestTag = "v0.0.0"
}

Write-Host "Current Latest Tag: $latestTag" -ForegroundColor Cyan

# Parse SemVer (vX.Y.Z)
if ($latestTag -match '^v?(\d+)\.(\d+)\.(\d+)') {
    [int]$major = $Matches[1]
    [int]$minor = $Matches[2]
    [int]$patch = $Matches[3]
} else {
    $major = 0; $minor = 0; $patch = 0
}

# 3. Determine new version
$newTag = ""
switch -Regex ($Type.ToLower()) {
    "^patch$" {
        $patch++
        $newTag = "v$major.$minor.$patch"
    }
    "^minor$" {
        $minor++
        $patch = 0
        $newTag = "v$major.$minor.$patch"
    }
    "^major$" {
        $major++
        $minor = 0
        $patch = 0
        $newTag = "v$major.$minor.$patch"
    }
    "^v?\d+\.\d+\.\d+" {
        if ($Type.StartsWith("v")) {
            $newTag = $Type
        } else {
            $newTag = "v$Type"
        }
    }
    Default {
        Write-Host "Invalid version type: '$Type'. Use: patch, minor, major, or a version string like v1.1.0" -ForegroundColor Red
        exit 1
    }
}

if ($Message -eq "") {
    $Message = "Release $newTag"
}

Write-Host "Bumping version: $latestTag -> $newTag" -ForegroundColor Green

# 4. Create git tag
git tag -a $newTag -m $Message
if ($LASTEXITCODE -eq 0) {
    Write-Host "Created Git Tag: $newTag" -ForegroundColor Green
    Write-Host "To publish release on GitHub, run:" -ForegroundColor Yellow
    Write-Host "   git push origin $newTag" -ForegroundColor Cyan
} else {
    Write-Host "Failed to create tag $newTag" -ForegroundColor Red
}
