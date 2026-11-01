# Tethr

Tethr is a free, open-source service that keeps track of what you watch, in whichever player you watch it. Media players send it what you play, and you get your history, what's next, and skip markers that fit the exact copy you're watching.

It's compatible with the Trakt API, so an app that already supports it can switch by changing its server address.

It's early. Nothing here is ready to use yet.

## Running it locally

You need Go and Docker.

```
docker compose up -d
make test
make run
```

The server listens on `localhost:8080`.

## Contributing

Every change, including the maintainer's, goes through a pull request and has to pass the checks before it's merged. Bug reports and ideas are welcome as issues. To report a security problem, see `SECURITY.md`.

## Licences

The server is licensed under the GNU Affero General Public License v3.0. See `LICENSE`. If you run a modified copy for other people, you have to share your changes.

Tethr's API spec and client libraries are MIT licensed, so any app can use them. The skip marker data is published under the Open Database License.

Copyright © 2026 Ellisn Ltd and contributors.
