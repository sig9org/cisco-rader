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
by default; set `settings.headless: true` to run without displaying its UI.

## Installation

```sh
go install github.com/sig9org/cisco-rader@latest
```

Release binaries are also available from the
[GitHub releases page](https://github.com/sig9org/cisco-rader/releases).

## Configuration

Create `config.yml` in the working directory. It contains the monitored sites,
runtime options, and chat notification destinations.

```yaml
settings:
  mention: user@example.com
  separate: false
  threads: 1
  headless: false
  user-agent: ""
  timeout: 45s
  silent: false
  debug: false

notifications:
  teams:
    destination: https://example.invalid/teams-webhook
  webex:
    token: ""
    destination: ""
  slack:
    destination: ""
    token: ""
    channel: ""

sites:
  - name: Cisco Catalyst 9800 Wireless Controller
    url: https://software.cisco.com/download/home/285968390/type/286278832/release/
```

The `settings.mention` value accepts the identifier format supported by
chatxgo, such as an email address for Microsoft Teams or Cisco Webex. Set
`settings.threads` to the number of URLs to check concurrently; the default is
`1` and values must be greater than zero. The `notifications` values replace the former `config.ini` keys: `teams.destination`
is `MSTEAMS_DST`, `webex.token` and `webex.destination` are `WEBEX_TOKEN` and
`WEBEX_DST`, and the corresponding Slack fields map to `SLACK_*`. `proxy` maps
to `PROXY`.

Use `-config` to choose another YAML file. The state file is derived from its
name, so `config.yml` uses `config_state.yml` in the same directory.

Keep `config.yml` private because it can contain credentials. The repository's
`.gitignore` excludes it. `config.yml.example` is provided as a template.

## Usage

```text
cisco-rader [flags]

      -config string      configuration file (default: config.yml)
      -no-notify          do not send chat notifications when releases change
      -no-save            do not write the derived *_state YAML file
      -init               recreate the state file from the current site values
      -dryrun             show the planned operation without fetching, notifying, or saving
      -update             update cisco-rader to the latest release and exit
  -v, -version            print version information and exit
  -h, -help               show the help message and exit
```

The first successful check establishes the initial state without notifying.
Later release changes trigger notifications unless `-no-notify` is set.
`-no-save` prevents state changes from being written. `-dryrun` performs no
browser access, notification, or state write. `-init` ignores the existing
state file and recreates it from the current site values without sending
initial-state notifications.

By default, all changed software pages are grouped into one chat message. The
subject includes the number of updated pages, such as
`Cisco Software Update (2 updates)`. Software names are shown as normal-sized
bold text in the body. With `-separate`, each changed page is sent as a
separate message whose subject is only the corresponding `name` from
`config.yml`. Set `settings.separate: true` to enable this behavior.

All runtime log messages include a timestamp. Normal output is uncolored,
warnings are orange, errors are red, and debug messages are gray and include a
`[DEBUG]` marker. The `settings.debug` option overrides `settings.silent`.

## Development

```sh
go test ./...
go build ./...
```

Version output uses the nearest Git tag, such as `v0.0.2`, without displaying a
commit hash. Release builds can set `internal/version.Version` with Go linker
flags. `-update` downloads the latest compatible release from
`sig9org/cisco-rader`.

## License

[MIT](https://github.com/sig9org/cisco-rader/blob/main/LICENSE)
