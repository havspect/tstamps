# Install the CLI

`tstamps` is a single binary. It talks to a running server over the HTTP API and never opens the database, so it needs no Go toolchain, no SQLite and no configuration beyond a server URL and a token.

The server is Linux-only. The CLI ships for Linux, macOS and Windows.

## Download

Take the archive for your machine from the [latest release](https://github.com/havspect/tstamps/releases/latest):

| Platform | Assets |
| --- | --- |
| Linux | `tstamps_linux_amd64.tar.gz`, `tstamps_linux_arm64.tar.gz` |
| macOS | `tstamps_darwin_amd64.tar.gz` (Intel), `tstamps_darwin_arm64.tar.gz` (Apple silicon) |
| Windows | `tstamps_windows_amd64.zip` |

Each release also publishes `checksums.txt`.

::: code-group
```sh [Linux]
# amd64 shown; use ARM64 on an arm64 machine.
curl -fsSL https://github.com/havspect/tstamps/releases/latest/download/tstamps_linux_amd64.tar.gz | tar -xz
sudo install -m 0755 tstamps /usr/local/bin/tstamps
tstamps version
```

```sh [macOS]
# arm64 shown; use amd64 on an Intel Mac.
curl -fsSL https://github.com/havspect/tstamps/releases/latest/download/tstamps_darwin_arm64.tar.gz | tar -xz
sudo install -m 0755 tstamps /usr/local/bin/tstamps
tstamps version
```

```powershell [Windows]
$dir = "$env:LOCALAPPDATA\tstamps"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
Invoke-WebRequest -Uri "https://github.com/havspect/tstamps/releases/latest/download/tstamps_windows_amd64.zip" -OutFile "$dir\tstamps.zip"
Expand-Archive -Path "$dir\tstamps.zip" -DestinationPath $dir -Force
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
[Environment]::SetEnvironmentVariable("Path", "$userPath;$dir", "User")
& "$dir\tstamps.exe" version
```
:::

On Windows, open a new terminal before using `tstamps` so the updated `Path` is loaded.

## Verify the download

`checksums.txt` lists a SHA-256 for every asset. Compare the file you downloaded with the line for your asset.

::: code-group
```sh [Linux]
sha256sum --ignore-missing -c checksums.txt
```

```sh [macOS]
shasum -a 256 -c checksums.txt
```

```powershell [Windows]
Get-FileHash .\tstamps_windows_amd64.zip -Algorithm SHA256
```
:::

## Log in

The CLI stores a personal API token after a password login. It does not keep your password.

::: code-group
```sh [Linux]
tstamps login --url https://time.example.com/w/example.com
```

```sh [macOS]
tstamps login --url https://time.example.com/w/example.com
```

```powershell [Windows]
tstamps login --url https://time.example.com/w/example.com
```
:::

It prompts for your email and password, then prints `logged in as Name <email>`. `tstamps whoami` shows the account and every team you belong to.

The URL names the workspace:

```sh
tstamps login --url https://time.example.com/w/example.com
```

The config file stores that URL, so every later command stays in the workspace. Log in again with another URL to switch. See [Hosted workspaces](./workspaces.md).

::: warning Initial password
A user created or reset by an admin must sign in to the web UI and change the initial password first. Until then the API refuses everything except the password-change endpoints, so `tstamps login` cannot mint a token.
:::

## Where the token is kept

| Platform | Config file |
| --- | --- |
| Linux | `~/.config/tstamps/config.json` |
| macOS | `~/.config/tstamps/config.json` |
| Windows | `%USERPROFILE%\.config\tstamps\config.json` |

The file holds a bearer token and is written with mode `0600` on Linux and macOS. Windows does not apply Unix modes; treat the file as a credential.

The path is literal: on macOS it is not under `~/Library/Application Support`, and on Windows it is not under `%APPDATA%`.

Settings resolve in this order, first match wins:

1. `--url` / `--token` on the command
2. `TSTAMPS_URL` / `TSTAMPS_TOKEN`
3. the config file

## Update the CLI

Download the new archive and replace the binary. The token stays valid.

::: code-group
```sh [Linux]
curl -fsSL https://github.com/havspect/tstamps/releases/latest/download/tstamps_linux_amd64.tar.gz | tar -xz
sudo install -m 0755 tstamps /usr/local/bin/tstamps
tstamps version
```

```sh [macOS]
curl -fsSL https://github.com/havspect/tstamps/releases/latest/download/tstamps_darwin_arm64.tar.gz | tar -xz
sudo install -m 0755 tstamps /usr/local/bin/tstamps
tstamps version
```

```powershell [Windows]
$dir = "$env:LOCALAPPDATA\tstamps"
Invoke-WebRequest -Uri "https://github.com/havspect/tstamps/releases/latest/download/tstamps_windows_amd64.zip" -OutFile "$dir\tstamps.zip"
Expand-Archive -Path "$dir\tstamps.zip" -DestinationPath $dir -Force
tstamps version
```
:::

## Uninstall

Remove the binary and, if you want to forget the server and token, delete the config file above.
