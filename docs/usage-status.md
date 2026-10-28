# Historical model usage

The native `usage_status` tool reads the latest shared usage ledger. It does not call a model or change routing. `routing_status` remains the monthly budget and model-routing tool. The `/usage` command remains a summary of the current UTC month.

Supply `start` and `end` as RFC3339 timestamps with `Z` or numeric timezone offsets. The start is inclusive; the end is exclusive. Date-only inputs and reversed or empty windows are errors. Resolve relative dates in the user's timezone and state the exact window in the answer. A rolling three-day window ends now; three complete calendar days end at today's local midnight. These windows differ, so use the user's requested meaning.

The response normalizes boundaries to UTC and reports calls, input tokens, output tokens and total tokens, overall and by pool. Token totals include only calls with reported usage. Missing usage and missing prices are separate counts. Estimated cost sums only records where both usage and price are known. `cost_incomplete` is true if either count is nonzero. Zero configured prices remain valid known prices. Empty windows have zero totals and an empty pool list. Ledger read errors are errors, never stale or zero reports. No prices are inferred or fetched.

This ledger covers calls recorded by this native runtime and other runtimes sharing its ledger. It does not measure unrelated provider, CLI or account-wide activity. Responses contain aggregate counts and configured pool names; they do not expose prompts, run IDs, model histories or credentials.

## Building a future image from reconciled source

Both Dockerfiles compile the current build context, including `usage_status` and the reconciled native fixes. The Mac checkout also contains newer compaction and worker-discovery work. The initial narrow native update deliberately uses the exact installed source plus the usage delta; its source manifest is separate from this combined checkout. Never build the stale clone in the running container as a substitute.

Both Dockerfiles provide a `binary-export` target. This builds only the required compiler stages and exports `iugum`; it does not build the runtime desktop, install agent CLIs, recreate a container or change a running service. For the portable static native binary use the root Dockerfile with `CGO_ENABLED=0`. The agentbox target uses CGO and the desktop's ICU/libc. Supply the existing private-module BuildKit secret through the established mechanism; do not put its contents in arguments or source.

Example from a reviewed source snapshot at the repository root:

```sh
docker buildx build --platform linux/amd64 --target binary-export \
  --build-arg CGO_ENABLED=0 \
  --build-arg SOURCE_REVISION=BASE-dirty-MANIFEST_SHA256 \
  --build-arg BUILD_DATE=UTC_BUILD_TIMESTAMP \
  --output type=local,dest=/tmp/iugum-binary-export .
```

Replace the provenance placeholders with the actual base revision, complete snapshot manifest digest and UTC build time. Record the full file manifest, exact command, compiler/base versions and output SHA256 with the release. A base commit alone cannot identify a dirty checkout. The `SOURCE_REVISION` and `BUILD_DATE` arguments are also supported by `agentbox/Dockerfile`. Exporting a binary is separate from an approved atomic native-service replacement; retain the prior release and restart only the native supervisor child. A full future image build naturally uses the same reconciled source context.
