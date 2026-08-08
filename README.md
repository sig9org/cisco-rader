<p align="center">
  <img src="https://raw.githubusercontent.com/sig9org/cisco-rader/main/assets/logo.webp" alt="cisco-rader">
</p>

# cisco-rader

[![Go Reference](https://pkg.go.dev/badge/github.com/sig9org/cisco-rader.svg)](https://pkg.go.dev/github.com/sig9org/cisco-rader)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

`cisco-rader` monitors Cisco Software Download pages and reports changes to
Suggested Release and Latest Release versions. It stores the last observed
versions in YAML and can notify Cisco Webex, Microsoft Teams, and Slack through
[chatxgo](https://github.com/sig9org/chatxgo).

## Requirements

- Go 1.26.5 or later when building from source
- Chrome or Chromium and, by default, an active desktop session

The Cisco download site is rendered in a browser. The browser window is shown
by default; use `-headless` to run without displaying its UI.

## Installation

```sh
go install github.com/sig9org/cisco-rader@latest
```

Release binaries are also available from the
[GitHub releases page](https://github.com/sig9org/cisco-rader/releases).

## Site configuration

Create `sites.yml` in the working directory. If it is absent, `site.yaml` is
used instead.

```yaml
sites:
  - name: Cisco Catalyst 9800 Wireless Controller
    url: https://software.cisco.com/download/home/285968390/type/286278832/release/
```

An optional `settings.mention` value adds a chat mention to release-change
notifications. It accepts the identifier format supported by chatxgo, such as
an email address for Microsoft Teams or Cisco Webex:

```yaml
settings:
  mention: user@example.com
```

Use `-site` to choose another file. The state file is derived from its name:
`sites.yml` uses `sites_state.yml`, and `-site mysite.yml` uses
`mysite_state.yml` in the same directory.

## Chat notification configuration

Copy `config.ini.example` to `config.ini`. Each INI section is a named profile,
and every non-empty `*_DST` entry enables that chat service.

```ini
[default]
MSTEAMS_DST=https://example.invalid/teams-webhook

[webex]
WEBEX_TOKEN=your-token
WEBEX_DST=your-room-id

[all]
MSTEAMS_DST=https://example.invalid/teams-webhook
WEBEX_TOKEN=your-token
WEBEX_DST=your-room-id
```

The `default` profile is selected automatically. Use `-profile webex` to choose
another section, and `-config` to choose another INI file. Without `-config`,
lookup follows chatxgo: `./config.ini` first, then
`~/.config/chatxgo/config.ini` on Linux/macOS or the per-user roaming config
directory on Windows. If neither file exists, chatxgo environment variables
are used for the default profile.

Keep `config.ini` private because it can contain credentials. The repository's
`.gitignore` excludes it.

## Usage

```text
cisco-rader [flags]

      -site string        site configuration file (default: sites.yml, then site.yaml)
      -config string      chat configuration file (default: config.ini lookup)
      -profile string     chat configuration profile (default: "default")
      -separate           send each software update as a separate message
      -headless           run Chrome or Chromium without displaying its UI
      -user-agent string  browser User-Agent (default: detected Chrome User-Agent)
      -timeout duration   release information retrieval timeout (default: 45s)
      -no-notify          do not send chat notifications when releases change
      -no-save            do not write the derived *_state YAML file
      -dryrun             show the planned operation without fetching, notifying, or saving
      -silent             suppress normal stdout messages
      -debug              print timestamped debug tracing to stdout (overrides -silent)
      -update             update cisco-rader to the latest release and exit
  -v, -version            print version information and exit
  -h, -help               show the help message and exit
```

The first successful check establishes the initial state without notifying.
Later release changes trigger notifications unless `-no-notify` is set.
`-no-save` prevents state changes from being written. `-dryrun` performs no
browser access, notification, or state write.

By default, all changed software pages are grouped into one chat message. The
subject includes the number of updated pages, such as
`Cisco Software Update (2 updates)`. Software names are shown as normal-sized
bold text in the body. With `-separate`, each changed page is sent as a
separate message whose subject is only the corresponding `name` from
`sites.yml`.

All runtime log messages include a timestamp. Normal output is uncolored,
warnings are orange, errors are red, and debug messages are gray and include a
`[DEBUG]` marker. `-debug` overrides `-silent`.

## Development

```sh
go test ./...
go build ./...
```

Version output uses `git describe --tags` and includes the commit hash. Release
builds can set `internal/version.Version` and `internal/version.Commit` with Go
linker flags. `-update` downloads the latest compatible release from
`sig9org/cisco-rader`.

## License

[MIT](https://github.com/sig9org/cisco-rader/blob/main/LICENSE)
