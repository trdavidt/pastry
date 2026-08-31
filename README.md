# pastry

A pastebin.com CLI. Create pastes from stdin, and read, list, and delete them.

Uses pastebin.com's API: [https://pastebin.com/doc_api](https://pastebin.com/doc_api)

```
echo "hello world" | pastry              # create an untitled paste, prints URL
echo "hi" | pastry --title "greeting"    # create a titled paste
pastry https://pastebin.com/AbC12345     # read a paste to stdout
```

## Install

`.rpm`, and `.deb` packages are attached to
[GitHub Releases](https://github.com/trdavidt/pastry/releases).

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
  <command> | pastry                 create a paste from stdin
      -t, --title T       paste title
      -f, --format F      syntax highlighting (e.g. go, python)
      -e, --expire E      expiration: N, 10M, 1H, 1D, 1W, 2W, 1M, 6M, 1Y
      -g, --guest         force a guest paste (no user key)
      -p, --private       private paste (requires login)

  pastry <url-or-key>                read a paste
  pastry list [-l|--limit N]         list your pastes (login required)
  pastry delete <url-or-key>         delete a paste (login required)
  pastry whoami                      show account info (login required)
  pastry login                       obtain and store your api_user_key
  pastry logout                      remove your stored api_user_key
```

All pastes default to unlisted.

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


