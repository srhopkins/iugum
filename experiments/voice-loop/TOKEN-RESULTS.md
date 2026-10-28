# Token cost of private PTT drafts

Measured on the existing Home Lab model through LiteLLM, 2026-10-21 Phoenix time.

**Three completed drafts consumed almost three times the reported tokens of one
answer.** Promoting the ready final draft used no additional model request.

| Path | Model requests | Input tokens | Output tokens | Total tokens |
| --- | ---: | ---: | ---: | ---: |
| One answer to the final transcript | 1 | 3,411 | 89 | 3,500 |
| Three successive drafts, including the final transcript | 3 | 10,219 | 231 | 10,450 |
| Release of the matching final draft | 0 additional | 0 additional | 0 additional | 0 additional |

The repeated path used 2.986 times the tokens, an increase of 6,950 tokens.
Almost all of the overhead was repeated input context. The adopted final reply was the single word `READY`, yet that request
reported 65 output tokens. Across the three requests, the provider reported
65–92 output tokens. The current ledger does not split visible text from reasoning tokens.
Therefore visible reply length is not a reliable measure of generation cost.

## Method

The test used the running isolated native agent and the browser harness's fixed
proxy, without opening a microphone or calling tools. The final-only baseline
prepared one answer and cancelled it rather than saving a conversation turn.
Then three serial revisions simulated a growing transcript. Every preparation
checked hashes of native history and workspace files against the same starting
state. Both final prompts matched exactly; the dynamic timestamp remained fresh
on each normal model request. Conversation files stayed unchanged before release.

The three draft input counts were 3,403, 3,405 and 3,411. The few new dictation
words increased the input count slightly, but the whole normal context was
reported again on every request. The corresponding output counts were 92, 74
and 65. Their request durations were 13.4, 11.3 and 10.3 seconds. The final-only
baseline took 13.1 seconds. Final promotion took 4 ms and added no usage record.

This deliberately waits for each draft to finish. It is a controlled cost
comparison, not a real-time human hold or a claim that three drafts finish in a
short utterance. Browser code runs one preparation at a time and retains only
the latest waiting transcript. An earlier exploratory run with a less bounded
partial prompt was rejected with HTTP 409; its response reason was not captured.
It is not included in the successful comparison and is not used to infer cost.

## What the counters mean

The native runtime saves prompt and completion totals reported by the provider.
All measured records report known usage. The current ledger does not retain
cached-input details, reasoning-token details, or model prefill timing. Reported
input tokens can include cached tokens, so a threefold token count does not
establish threefold GPU work or threefold billing. Model prices are not configured;
the zero estimate in the ledger is not a measured dollar cost.

Failed and cancelled requests can consume compute without returning usage. These
totals describe completed responses; they are not a complete GPU-cost ledger.
The existing runtime also puts a changing timestamp near the start of the request.
That may reduce prefix-cache reuse; this is a source-based concern, not a measured
cache miss rate. No cache-hit claim is supported by this experiment.

## Recommendation

Keep this as an optional experiment. Do not repeatedly generate complete answers
for every transcript update in the production Chat extension. First measure and
improve draft reuse: require meaningful text changes, bound preparation work,
prefer one useful draft, and evaluate incremental generation with explicit
cancellation. Preserve fresh time context while investigating whether its
placement prevents useful prefix caching. Measure both wasted tokens and time
saved after release before enabling paid models or general rollout.

Raw results are in `token-measurements.json`. The older latency measurements and
full prototype limitations remain in `RESULTS.md`. No production configuration,
model settings, active conversation files or installed extension were changed.
