# pastry

A pastebin.com CLI. Create pastes from stdin, and read, list, and delete them.

```
echo "hello world" | pastry              # create an untitled paste, prints URL
echo "hi" | pastry --title "greeting"    # create a titled paste
pastry https://pastebin.com/AbC12345     # read a paste to stdout
```

## Install

Prebuilt static Linux binaries, `.rpm`, and `.deb` packages are attached to
[GitHub Releases](https://github.com/<owner>/pastry/releases).

```sh
# From a release tarball
curl -L https://github.com/<owner>/pastry/releases/latest/download/pastry_linux_amd64.tar.gz | tar xz

# From source (requires Go 1.26+)
go install ./...
```

## Setup

The pastebin API requires a developer API key even for guest pastes. On first
use, `pastry` prompts for it and stores it in `~/.config/pastry/config.json`
(mode `0600`). Get your key by logging in at <https://pastebin.com/doc_api>.

```sh
pastry login        # optionally log in to enable account features
pastry logout       # remove the stored user key
```

## Usage

```
<command> | pastry                  create a paste from stdin
<command> | pastry --title T        create a titled paste
pastry <url-or-key>                 read a paste
pastry list [-l|--limit N]          list your pastes (login required)
pastry delete <url-or-key>          delete a paste (login required)
pastry whoami                       show account info (login required)
pastry login                        obtain and store your api_user_key
pastry logout                       remove your stored api_user_key
```

### Create options

| Option | Description |
| --- | --- |
| `-t`, `--title T` | Paste title |
| `-f`, `--format F` | Syntax highlighting value (e.g. `go`, `python`) |
| `-e`, `--expire E` | Expiration: `N`, `10M`, `1H`, `1D`, `1W`, `2W`, `1M`, `6M`, `1Y` |
| `-g`, `--guest` | Force a guest paste (no user key, even if logged in) |
| `-p`, `--private` | Private paste (requires login) |

All pastes default to **unlisted**.

## Configuration

Credentials live in `~/.config/pastry/config.json`:

```json
{
  "api_dev_key": "...",
  "api_user_key": ""
}
```

`pastry login` populates `api_user_key`; `pastry logout` clears it. Account
features (list, delete, whoami, private pastes) require the user key.

## Development

```sh
go build ./...   # build
go test ./...    # test
go vet ./...     # vet
gofmt -l .       # check formatting
```

CI runs `gofmt`, `go vet`, `go test`, and `go build` on every push/PR. Tagging
a release (`v*`) triggers GoReleaser to build binaries and `.rpm`/`.deb`
packages.
