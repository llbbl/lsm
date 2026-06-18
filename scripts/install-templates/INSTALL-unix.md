# Installing lsm

This archive contains the `lsm` binary, this guide, and the project `LICENSE`.

## 1. Extract

If you have not already:

```sh
tar xzf lsm-*-*.tar.gz
```

This unpacks `lsm`, `INSTALL.md`, and `LICENSE` into the current directory.

## 2. Make it executable

```sh
chmod +x lsm
```

## 3. Move it onto your PATH

Pick a directory that is already on your `PATH`. Common choices:

```sh
# Per-user (no sudo). Create the dir if it does not exist.
mkdir -p "$HOME/.local/bin"
mv lsm "$HOME/.local/bin/"

# ...or system-wide (requires sudo)
sudo mv lsm /usr/local/bin/
```

If you used `~/.local/bin` and it is not on your `PATH`, add this to your shell
profile (`~/.zshrc`, `~/.bashrc`, etc.):

```sh
export PATH="$HOME/.local/bin:$PATH"
```

## macOS Gatekeeper note

macOS quarantines binaries downloaded from the internet. If you see
"cannot be opened because the developer cannot be verified", clear the
quarantine attribute:

```sh
xattr -d com.apple.quarantine lsm
```

(Run this before moving the binary, or against its final path.)

## 4. Verify

```sh
lsm --version
```

## Quick start

```sh
# Generate your age encryption key
lsm init

# Register the current project directory as an app
cd ~/Web/myapp
lsm link myapp
```

## More

Documentation and source: https://github.com/llbbl/lsm
