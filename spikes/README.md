# EdgeSync spikes

Throwaway proof-of-concept code for the Phase 0 spikes. This directory is a separate Go module
(`github.com/kikz0r/edgesync/spikes`) and is not part of the main module's gates.

## Rules (D-74)

- Spike code lives in `spikes/` as a separate Go module (`github.com/kikz0r/edgesync/spikes`) excluded from the main module's gates, linted only with `go vet` and `go build`.
- Spike binaries are run by the user following `qa/manual/p0-*.md` scripts (never by agents); automatable checks are tests in the spikes module (loopback-only tests may run on the host; anything touching real input/clipboard/display/multicast is `integration`-tagged).
- Each spike stream writes `briefs/spikes/<stream>.md` with a go/no-go per mechanism.
- A spike result contradicting a locked decision stops the stream: the parent asks the user before anything changes.
- The spikes module is deleted in P5 (item in `briefs/plan-phase-5.md`), after its findings are archived.

## Layout conventions

- Every spike directory has its own `README.md` with these sections: purpose, exact build command, exact run command(s), what to observe, what to paste back.
- Built binaries go into `spikes/bin/` (gitignored).
- Outputs go into `spikes/out/` (gitignored; uploaded as a CI artifact by `.github/workflows/spikes.yml`).

## Host gates

- `go -C spikes vet ./...`
- `go -C spikes build ./...`
- `go -C spikes test ./<touched dirs>/...` (host-safe tests only)
