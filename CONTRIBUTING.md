# Contributing to Tethr

Thanks for helping. The README explains how to run Tethr locally. You need Go, Docker, Node and pnpm.

## Finding something to work on

Open issues are grouped on the [project board](https://github.com/orgs/tethrtv/projects/1). Issues labelled `good first issue` are small and self-contained. Anything in the "Before invites" milestone is what matters most right now. For anything bigger than a small fix, say on the issue that you're picking it up before you start, so two people don't build the same thing.

## Making a change

Keep each pull request to one change, and link the issue it works on in the description, like `closes #12`. The checks have to pass before it's merged. You can run most of them yourself with `make test` and `make lint`, and with `pnpm check`, `pnpm test` and `pnpm build` in `web/`.

Match the style of the code around your change. Go is formatted with `gofmt`, and the website with Biome.

Tethr's own logic lives in `internal/core` and is tested against a real Postgres, which `docker compose up -d` starts. `internal/api` is the native API and `internal/traktcompat` translates Trakt-style requests. Both call into core.

API changes start in the spec, `spec/tethr/openapi.yaml`. Then run `make gen` for the server and `pnpm gen` in `web/` for the website, because the checks fail if generated code is out of date.

## One rule about data

The API returns IDs, numbers and people's own data, never content from TMDB such as titles, synopses or images. Apps fetch those from TMDB with their own keys, and titles and years come from Wikidata. Please don't add anything that puts TMDB content in API responses. The one exception is `/meta`, outside `/v1`, which only Tethr's own website calls.

## Licence

By contributing you agree that your work is released under the same licence as the part of the repository it's in: MIT for the API spec in `spec/tethr/`, and AGPL-3.0 for the rest. The Trakt spec snapshot in `spec/trakt/` keeps Trakt's own MIT licence.
