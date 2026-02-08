# macos-login-items-cli

`mlogin` is a Go CLI for macOS that gives you scriptable visibility and control over:

- Login items (what starts when you log in)
- Launchd background items (LaunchAgents / LaunchDaemons)
- System extensions (`systemextensionsctl list`)
- An interactive terminal UI (TUI) for browsing and quick actions

It is designed as a practical CLI-first alternative to manually navigating Settings.

## Build

```bash
go build -o mlogin ./cmd/mlogin
```

## Version

```bash
./mlogin version
```

Release builds inject:
- semantic version from git tag (for example `v1.2.3`)
- commit SHA
- build date

## Usage

### Interactive TUI

```bash
./mlogin tui
```

TUI controls:

- `tab` switch Login/Background/System Extensions tabs
- `r` refresh
- `/` search/filter items
- `c` clear filter
- `x` delete selected login item (Login tab)
- `e` / `d` enable/disable selected background item (Background tab)
- `x` on Background tab prompts to permanently delete selected background item
- `y` / `n` confirm or cancel destructive prompts
- `q` quit

### Login items

List login items:

```bash
./mlogin login list
./mlogin login list --json
```

Add login item:

```bash
./mlogin login add --path /Applications/SomeApp.app
./mlogin login add --path /Applications/SomeApp.app --hidden
```

Remove login item:

```bash
./mlogin login remove --name "SomeApp"
./mlogin login remove --path /Applications/SomeApp.app
```

### Background items (launchd)

List known agents/daemons:

```bash
./mlogin background list
./mlogin background list --scope user
./mlogin background list --scope system
./mlogin background list --json
```

Enable/disable label:

```bash
./mlogin background enable --label com.example.agent --scope user
./mlogin background disable --label com.example.agent --scope user
```

Load/unload service:

```bash
./mlogin background load --plist ~/Library/LaunchAgents/com.example.agent.plist --scope user
./mlogin background unload --label com.example.agent --scope user
```

Delete service and plist file:

```bash
./mlogin background delete --label com.example.agent --plist ~/Library/LaunchAgents/com.example.agent.plist --scope user
```

### System extensions

List system extensions:

```bash
./mlogin extensions list
./mlogin extensions list --json
```

## Notes

- `login` commands use `osascript` with JavaScript for Automation against `System Events`.
- `background` commands wrap `launchctl`.
- `--scope system` usually requires `sudo`.
- macOS app-level "Allow in Background" toggles from System Settings are partially represented through launchd services and may vary by app implementation.

## CI, Release, and Homebrew Tap

This repository includes:
- CI workflow: `.github/workflows/ci.yml`
- Tag-based release workflow: `.github/workflows/release.yml`
- GoReleaser config: `.goreleaser.yaml`

### Versioning model

- Use semantic version tags: `vMAJOR.MINOR.PATCH` (example: `v1.0.0`).
- Pushing a semver tag triggers the release workflow.
- The release workflow runs tests, creates macOS binaries (`darwin/amd64` and `darwin/arm64`), publishes a GitHub Release, and updates the Homebrew tap formula.

Create and push a release tag:

```bash
git tag v0.1.0
git push origin v0.1.0
```

### Homebrew tap setup

1. Create a tap repository (for example `j4n-e4t/homebrew-tap`) with a `Formula/` directory.
2. Add a repo secret in this repo:
   - `HOMEBREW_TAP_GITHUB_TOKEN`: GitHub token with write access to the tap repo.
3. If your tap repo differs from `j4n-e4t/homebrew-tap`, update values in `.github/workflows/release.yml`:
   - `HOMEBREW_TAP_OWNER`
   - `HOMEBREW_TAP_REPO`

After the first release, users can install with:

```bash
brew tap j4n-e4t/tap
brew install j4n-e4t/tap/mlogin
```
