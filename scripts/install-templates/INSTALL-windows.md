# Installing lsm (Windows)

This archive contains the `lsm.exe` binary, this guide, and the project `LICENSE`.

## 1. Unzip

Extract the `.zip` archive (right-click > Extract All, or use your preferred
tool). You will get `lsm.exe`, `INSTALL.md`, and `LICENSE`.

## 2. Move it onto your PATH

Move `lsm.exe` to a directory on your `PATH`. A common per-user choice:

```powershell
# Create a local bin dir if you do not have one
New-Item -ItemType Directory -Force -Path "$HOME\bin"
Move-Item .\lsm.exe "$HOME\bin\lsm.exe"
```

If that directory is not already on your `PATH`, add it (per-user, no admin):

```powershell
[Environment]::SetEnvironmentVariable(
  "Path",
  [Environment]::GetEnvironmentVariable("Path", "User") + ";$HOME\bin",
  "User"
)
```

Open a new terminal afterwards so the updated `PATH` takes effect.

## 3. Verify

```powershell
lsm --version
```

## Quick start

```powershell
# Generate your age encryption key
lsm init

# Register the current project directory as an app
cd C:\Web\myapp
lsm link myapp
```

## More

Documentation and source: https://github.com/llbbl/lsm
