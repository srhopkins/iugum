<!-- <atomdown version="1"/> -->
<!-- <atom id="BA8Z4YHJ" slug="running-todo" digest="sha256:c91bf532f29004da559d52ccea526566abeb046e7f27a1d85ad61f27cc418c17"/> -->
# Running todo

<!-- <atom id="67FJXQS1" slug="the-master-list-daily-pages-are-filled" digest="sha256:a416cb8a06db2391c3fcc485481575192696ddf56e5e89d9a84e5c97aacb53ee"/> -->
The master list. Daily pages are filled from here, grouped by subject. This is a generated fixture: the shape of a real page, none of its content.

<!-- <atom-group id="KATZ94NM" slug="decisions"> -->

<!-- <atom id="VSECXNV6" slug="decisions-waiting-on-me" digest="sha256:c8466bfdd91679ccde3c03501d0cba2267a18c0071df17f33f98db89c49023ac"/> -->
## Decisions waiting on me

<!-- <atom id="E2AB4V7D" slug="read-this-group-first-nothing-below-is" digest="sha256:20c3e5c38d903de4625aa926292a8d4fe111a249bfbc3fcd2e2afcf952464ccb"/> -->
Read this group first. Nothing below is blocked on work. It is all blocked on an answer.

<!-- <atom id="10WYM84W" slug="1-history-one-commit-is-still-local" digest="sha256:7da5d55cbbc13bc04b097b4f4046493208c6692638475f0cdaf6bb792249db47"/> -->
1. **History.** One commit is still local and later commits reverse it. Drop it, squash the pair, or push the contradiction, and write down which of the three you chose so the next reader does not have to work it out again.
2. **Stale ticket.** A closed ticket carries a title that now states the opposite of the rule it closed, so anyone who finds it by search reads the reverse of the decision and has no way to tell from the ticket alone.
3. **Rewritten policy.** An agent rewrote a conformance note to fit a change. Defensible, but read that paragraph before it is quoted anywhere, because the version in the history and the version on the page disagree.
4. **Editor core.** Approve a two-line change to the vendored editor, and the upstream pull request that carries it, or say which of the two you want held so the vendored tree and the upstream one do not drift apart.
5. **One file, not two.** This page exists twice. Collapse it to one, probably a symlink, and pick which of the two paths is the real one before anything else starts linking to whichever copy it found first.
6. **Five calls.** Listed in the next group. Those five gate the tickets underneath them, so the order they are answered in decides the order everything below can be started in, not just when it finishes.

<!-- <atom id="NED06WGM" slug="answer-the-six-above-in-order-do" digest="sha256:162768ae07a517d47ac758224020a0179d6c7e04937283477040a5bf0e2ade1c"/> -->
Answer the six above in order. Do not start below them.

<!-- </atom-group> -->

<!-- <atom-group id="NS67J8K5" slug="resea"> -->

<!-- <atom id="SBV4X9FC" slug="resea-tickets-due-tonight" digest="sha256:922c179e1d941e64fb60191784c823faa0c121c5d785510724eb193caa65744e"/> -->
## RESEA tickets - due tonight

<!-- <atom id="D9XK0D8B" slug="the-work-is-not-writing-tickets-it" digest="sha256:948176c260f9175feff7dcd90b922cd677386bef964c4f9c3f9abbe16d05ae7e"/> -->
The work is not writing tickets. It is posting a review call into the tracker. Nothing has been posted yet.

<!-- <atom id="HMNG795Q" slug="the-feature-reads-a-state-case-system" digest="sha256:c676fe912eaa7aefe84d9d4c37c7638796c34b09765e695883aabe5e0068dd23"/> -->
The feature reads a state case system through `GET /resea-status` and shows the steps in a carousel. Epic: [FFAI-62016 "Home: carousel and action plan status"](https://example.invalid/browse/FFAI-62016).

<!-- <atom id="8KVA3F8D" slug="ticket-state-tonight" digest="sha256:f556d2c39a77c2e0aab8ce63526a90d6f51077708699d3f3a892b9778b6e7794"/> -->
| Ticket | State | Tonight |
|---|---|---|
| [FFAI-72357 "Productionize the programs service"](https://example.invalid/browse/FFAI-72357) | On Hold | Add cache expiry, single flight, rate limit, error code, unique key |
| [FFAI-62020 "Participant API integration"](https://example.invalid/browse/FFAI-62020) | Triage | Add the dedupe key. Land the connector first |
| [FFAI-62017 "Carousel and status modal"](https://example.invalid/browse/FFAI-62017) | Triage | Add the per-tenant switch and the fall-back decision |
| [FFAI-72606 "Confirm the API contract points"](https://example.invalid/browse/FFAI-72606) | Triage | Add the five missing questions. Do not close |
| [FFAI-72356 "Participant identity: verify and roll out"](https://example.invalid/browse/FFAI-72356) | Triage | **Top blocker.** Move out of Triage, name a backfill owner |
| [FFAI-72629 "Spike: steps data model and tenant config"](https://example.invalid/browse/FFAI-72629) | In progress | Comment two reversals: row filtering dropped, config moved |
| [FFAI-62021 "Spike: multi-program extensibility"](https://example.invalid/browse/FFAI-62021) | Verification | Needs owners for the commercial check and the service name |
| [FFAI-72628 "Spike: print component"](https://example.invalid/browse/FFAI-72628) | Triage | Keep open. It gates the row below |
| [FFAI-62019 "[nice to have] Print Action Plan"](https://example.invalid/browse/FFAI-62019) | Triage | Keep. Blocked on the print spike |
| [FFAI-72342 "Close the connector window"](https://example.invalid/browse/FFAI-72342) | Triage | Land the connector before this closes |

<!-- <atom id="X7FB0B9B" slug="five-calls-that-gate-the-tickets" digest="sha256:f3f1725f11504b69e578b1b9be9889ac0e859898621fb9024b8c19d6357c9953"/> -->
### Five calls that gate the tickets

<!-- <atom id="J0KD8XQ9" slug="1-the-local-note-contradicts-itself-on" digest="sha256:3b0123a2a44ff305fa1cf91c953b7d2f15ee02cd36de1bde63e47f63c5accc33"/> -->
1. The local note contradicts itself on one ticket: one section says close it as superseded, another says never close it. Pick one.
2. A request for stored state plus an error flag reverses two written decisions.
3. One ticket asks the state to drop a field another team expects to use. Settle it first.
4. Does the new admin API own the label overrides, or take upstream titles?
5. Does the P1 stay one ticket, or split now.

<!-- </atom-group> -->

<!-- <atom-group id="QP41ZR8T" slug="editor"> -->

<!-- <atom id="7WSE6GJ2" slug="editor-2" digest="sha256:90dfdc74de8ec936a54de91d1f15e94538e96a727b44c0de9d88bcae2a830b83"/> -->
## editor

<!-- <atom id="YYJNZETW" slug="note-0-editor-holds-at-the-current" digest="sha256:14d32e5156be6c1abbed3e8e626363050ba7d7ad609c4182f531ec5f22904234"/> -->
Note 0. editor holds at the current step. Nothing is blocked.

<!-- <atom id="H3GXEGB7" slug="check-1-read-editor-step-1-md" digest="sha256:35a85405e1cea9c6201dff5371eaaa0a2a914e63f4c24ce9c762997a3e027ed0"/> -->
Check 1. Read `editor/step-1.md` before the next change. The path is relative to the space root.

<!-- <atom id="R437KV5K" slug="editor-item-2a-it-is-written-long" digest="sha256:3fde2fab9583cc4d0de22832bbee257f703e17bd5b5731c7e2cd88db4b9fca74"/> -->
- editor item 2a. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of editor step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- editor item 2b. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of editor step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 2b1. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of editor step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 2b2. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of editor step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- editor item 2c. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of editor step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- <atom id="8FJAARPY" slug="a-quoted-line-for-editor-step-3" digest="sha256:d1b19b9f0bcc713894d55664131bd1f01952220b03caf72380c3ec162f9bd4cb"/> -->
> A quoted line for editor, step 3. It wraps far enough to make the blockquote bar measurable against the card border. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of editor step 3 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- <atom id="XVBQXX57" slug="editor-step-4" digest="sha256:004b4db253805278a06bdb140d582ecb0572f79aa49397b0196ae03e373f4fa8"/> -->
### editor step 4

<!-- </atom-group> -->

<!-- <atom-group id="VD07KHM2" slug="parser"> -->

<!-- <atom id="PKDWF0RD" slug="parser-2" digest="sha256:52354ec7d497f980e2eae58d1e9921eaa44923773016922097c2cf6d7649324f"/> -->
## parser

<!-- <atom id="S73BEEED" slug="note-0-parser-holds-at-the-current" digest="sha256:f27a6072cbb763826fbbd8e4921116fa3e1196208d294aa576fde7f6d581992c"/> -->
Note 0. parser holds at the current step. Nothing is blocked.

<!-- <atom id="1BNNCYVR" slug="check-1-read-parser-step-1-md" digest="sha256:12a506773433a3b3d678ebb358272cc353388e04538b29b58d812bb410f10c47"/> -->
Check 1. Read `parser/step-1.md` before the next change. The path is relative to the space root.

<!-- <atom id="H8HPXWMM" slug="parser-item-2a-it-is-written-long" digest="sha256:7da435f35bd73ec32e64eeb74dc4724d8d64baadfa05c22e272ac5117c431ab1"/> -->
- parser item 2a. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of parser step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- parser item 2b. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of parser step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 2b1. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of parser step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 2b2. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of parser step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- parser item 2c. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of parser step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- <atom id="41CSA9MS" slug="a-quoted-line-for-parser-step-3" digest="sha256:e5a9903fc68e7eda83be338078c05dc90f4115ab6aa372ea33682ad5eda9161c"/> -->
> A quoted line for parser, step 3. It wraps far enough to make the blockquote bar measurable against the card border. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of parser step 3 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- <atom id="19GPFPQM" slug="parser-step-4" digest="sha256:99ddedf5eaf0a114ba587e7de3f96dc5da69b9dfb55307ee6d178480c24763a4"/> -->
### parser step 4

<!-- </atom-group> -->

<!-- <atom-group id="WX53BCN9" slug="board-view"> -->

<!-- <atom id="WVG0AT5T" slug="board-view-2" digest="sha256:e47f766962e8506c89940f8a88b4e1c8ff849f909293ae64a50f99840ec50b41"/> -->
## board view

<!-- <atom id="9MQ9E0D8" slug="note-0-board-view-holds-at-the" digest="sha256:26b01d82b944dd0d1e75bb0c850480935ab1bb19d2009b7b5b75c041fe22f67a"/> -->
Note 0. board-view holds at the current step. Nothing is blocked.

<!-- <atom id="6CMBGC0A" slug="check-1-read-board-view-step-1" digest="sha256:2e659246944cbed5754a78adc1a1a85102940ce849b0f95145b0b4968624f7dc"/> -->
Check 1. Read `board-view/step-1.md` before the next change. The path is relative to the space root.

<!-- <atom id="0MQS37P1" slug="board-view-item-2a-it-is-written" digest="sha256:e7ab755dcfe11501ffc2739a39461f5814f0d00b69243bc9e97d830bdcb97959"/> -->
- board-view item 2a. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of board-view step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- board-view item 2b. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of board-view step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 2b1. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of board-view step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 2b2. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of board-view step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- board-view item 2c. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of board-view step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- <atom id="5JW12GFY" slug="a-quoted-line-for-board-view-step" digest="sha256:5c6035143eecdec8da4b8bd87ee7da5b8996dc8ef6cbb3c48ad592ee01855c00"/> -->
> A quoted line for board-view, step 3. It wraps far enough to make the blockquote bar measurable against the card border. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of board-view step 3 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- <atom id="8ZPS6P5F" slug="board-view-step-4" digest="sha256:e10fc90262bf0727125031b98c95253ffb8a2f006803344cfa8fb3600d7f16d3"/> -->
### board-view step 4

<!-- </atom-group> -->

<!-- <atom-group id="YT26FGK4" slug="inline-view"> -->

<!-- <atom id="HTJ02ZBN" slug="inline-view-2" digest="sha256:83d7d6e7b9633c305e5dbf9987434d2329ee0d0bb4cc2c6539efef9c3a006d2c"/> -->
## inline view

<!-- <atom id="0P6QG13Q" slug="note-0-inline-view-holds-at-the" digest="sha256:934c6e0a0ed14e52842afe94d02b0e6a5ce90e1c5188e04cf5939d8fa2fc6d64"/> -->
Note 0. inline-view holds at the current step. Nothing is blocked.

<!-- <atom id="716RWNT0" slug="check-1-read-inline-view-step-1" digest="sha256:ab94e5b6063a985f0a69efb4ed075322a3fa7012c654170093c978edc3caa0d5"/> -->
Check 1. Read `inline-view/step-1.md` before the next change. The path is relative to the space root.

<!-- <atom id="815W066A" slug="inline-view-item-2a-it-is-written" digest="sha256:daab9dd9fd0af179b92370ed56353cc966f0e035fa9497236aec54f4a58d4380"/> -->
- inline-view item 2a. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of inline-view step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- inline-view item 2b. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of inline-view step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 2b1. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of inline-view step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 2b2. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of inline-view step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- inline-view item 2c. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of inline-view step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- <atom id="CN8S04JM" slug="a-quoted-line-for-inline-view-step" digest="sha256:f1c494ac595bae6132577df23c67dcd8ade524231084330b645f6de3f020b368"/> -->
> A quoted line for inline-view, step 3. It wraps far enough to make the blockquote bar measurable against the card border. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of inline-view step 3 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- <atom id="VGAPWG4C" slug="inline-view-step-4" digest="sha256:63bfcbfa7d9cc34eaad376f5b2dac2be53485eb9688178dc90c4298048c21af4"/> -->
### inline-view step 4

<!-- </atom-group> -->

<!-- <atom-group id="ZR89PSD1" slug="database"> -->

<!-- <atom id="B03XD6NA" slug="database-2" digest="sha256:021c8667b7dd26b8d6fd456eb044cca5d068bd42ae7df930d3bf27d9c3c43660"/> -->
## database

<!-- <atom id="M1CTFFD0" slug="note-0-database-holds-at-the-current" digest="sha256:2cc3218f845be9eea61fb81d5fc64bbe7851f10c693ba9c4c475cbca2e79749a"/> -->
Note 0. database holds at the current step. Nothing is blocked.

<!-- <atom id="G0WQWDT6" slug="check-1-read-database-step-1-md" digest="sha256:e05f703fed62b0ab190ec569317a0cf25b9171025020c24f5208b6be01400545"/> -->
Check 1. Read `database/step-1.md` before the next change. The path is relative to the space root.

<!-- <atom id="K20MZKKF" slug="database-item-2a-it-is-written-long" digest="sha256:f023132e33a30c0ce94a1ae695323c7a4d8da07f50020fd8f9035e9c62cf18de"/> -->
- database item 2a. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of database step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- database item 2b. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of database step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 2b1. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of database step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 2b2. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of database step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- database item 2c. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of database step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- <atom id="ZX2TGYGB" slug="a-quoted-line-for-database-step-3" digest="sha256:14ac2dfa13a12dd194c38ca40d05f43b22d3436d5cee4fb829a232d6c2f43008"/> -->
> A quoted line for database, step 3. It wraps far enough to make the blockquote bar measurable against the card border. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of database step 3 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- </atom-group> -->

<!-- <atom-group id="BH14QWM7" slug="delivery"> -->

<!-- <atom id="JSD0HQG7" slug="delivery-2" digest="sha256:fa65ec268ce62fe99b12b6f220decd90f9829907924e57953a88fe274beb5be5"/> -->
## delivery

<!-- <atom id="34JV84P0" slug="note-0-delivery-holds-at-the-current" digest="sha256:22c984bf567661a12f588edb73c32f7f68c4d8bb20a2111991c0d83a90e4e1db"/> -->
Note 0. delivery holds at the current step. Nothing is blocked.

<!-- <atom id="H41990T6" slug="check-1-read-delivery-step-1-md" digest="sha256:7ef35a2b7b5ade0ac51b6c44e7e0b1d26e7cd6493be779c42f9333f9b30ad63b"/> -->
Check 1. Read `delivery/step-1.md` before the next change. The path is relative to the space root.

<!-- <atom id="FY6FDGR7" slug="delivery-item-2a-it-is-written-long" digest="sha256:f0a6b4fe140fed7e7f57ad7db525ccdeef1307103c77d181725d8640d32f2508"/> -->
- delivery item 2a. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of delivery step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- delivery item 2b. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of delivery step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 2b1. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of delivery step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 2b2. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of delivery step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- delivery item 2c. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of delivery step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- <atom id="ABH7HYZH" slug="a-quoted-line-for-delivery-step-3" digest="sha256:0e786e516aa5b8906895e4050d149b3b82e4087dcdd1f3c5d88a4bd2c6ae8380"/> -->
> A quoted line for delivery, step 3. It wraps far enough to make the blockquote bar measurable against the card border. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of delivery step 3 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- </atom-group> -->

<!-- <atom-group id="CJ62MVX3" slug="agents"> -->

<!-- <atom id="QDXS7RW6" slug="agents-2" digest="sha256:f943e43d2b5d3148d498a29c5b56666f0a0918180db58a78aba4f04510409e44"/> -->
## agents

<!-- <atom id="CE99AE4B" slug="note-0-agents-holds-at-the-current" digest="sha256:c79a7d5af1b44e61b2d283a876ab0ea6f296dde93d27340bab86f82b28db7216"/> -->
Note 0. agents holds at the current step. Nothing is blocked.

<!-- <atom id="H8ME51KW" slug="check-1-read-agents-step-1-md" digest="sha256:7a84f27914146d30cc1243a80ed985c211ee5e5172e530ca8d724bacba6f982e"/> -->
Check 1. Read `agents/step-1.md` before the next change. The path is relative to the space root.

<!-- <atom id="VDTW8TBF" slug="agents-item-2a-it-is-written-long" digest="sha256:7e1352463e1e27ce20d414667b6fa6f3f10e34a9b039d045902bf1bcf52d1bac"/> -->
- agents item 2a. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of agents step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- agents item 2b. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of agents step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 2b1. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of agents step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 2b2. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of agents step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- agents item 2c. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of agents step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- <atom id="K4QZXS5F" slug="a-quoted-line-for-agents-step-3" digest="sha256:feeb0f41a94c51607665bc7c42b3551c6f154355947520d488aa0000b0a45905"/> -->
> A quoted line for agents, step 3. It wraps far enough to make the blockquote bar measurable against the card border. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of agents step 3 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- </atom-group> -->

<!-- <atom-group id="DK75NRT8" slug="notes"> -->

<!-- <atom id="81P647PC" slug="notes-2" digest="sha256:b8c01b61c7a3a84c2caa61d43ef946572b1cc7af9e856106726cb48c7d04f954"/> -->
## notes

<!-- <atom id="HW08G6RR" slug="note-0-notes-holds-at-the-current" digest="sha256:e04ae8185359cb2106ba9a6bb7d4f1eab8c2cf75bdff21f151e7e84569574f81"/> -->
Note 0. notes holds at the current step. Nothing is blocked.

<!-- <atom id="47C5911M" slug="check-1-read-notes-step-1-md" digest="sha256:26c2ab1da187617f36496f7465c8ff402b8a9d3e943c6e58bd1cdfa0e00a1961"/> -->
Check 1. Read `notes/step-1.md` before the next change. The path is relative to the space root.

<!-- <atom id="M11YWXVA" slug="notes-item-2a-it-is-written-long" digest="sha256:b25ed6b69d7b31cacf63b3cfa31cf22886e0f7f66ecea7d316588627c3bcc7f7"/> -->
- notes item 2a. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of notes step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- notes item 2b. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of notes step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 2b1. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of notes step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 2b2. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of notes step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- notes item 2c. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of notes step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- <atom id="A25DGBSH" slug="a-quoted-line-for-notes-step-3" digest="sha256:534ae5d126f7628f2e64977aa121c2cd346ba96b90bf84c45e376e53eab85d7b"/> -->
> A quoted line for notes, step 3. It wraps far enough to make the blockquote bar measurable against the card border. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of notes step 3 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- </atom-group> -->

<!-- <atom-group id="EK38HYZ6" slug="backlog"> -->

<!-- <atom id="0X12GSRJ" slug="backlog-2" digest="sha256:5da0f9d612dc4b483a57a7a5f3e73b55b04b1cde2093b152c75f6dac3113d4cc"/> -->
## backlog

<!-- <atom id="VGF2MYW2" slug="note-0-backlog-holds-at-the-current" digest="sha256:fadbb710a360eddb24c735e07114d8e212ce66d6bb58cd32943ac61c2a296703"/> -->
Note 0. backlog holds at the current step. Nothing is blocked.

<!-- <atom id="WWHS9ZBH" slug="check-1-read-backlog-step-1-md" digest="sha256:b549c8f66edaa050032af3e4cc5404ccb2ef2b9e5a80dc51e6037a159d13187e"/> -->
Check 1. Read `backlog/step-1.md` before the next change. The path is relative to the space root.

<!-- <atom id="YK9JN6FA" slug="backlog-item-2a-it-is-written-long" digest="sha256:0f0cb4fe4ae915d3480ca0d3e36b9d6310c23dee52f2cebaaa0a71ca43e1e467"/> -->
- backlog item 2a. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of backlog step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- backlog item 2b. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of backlog step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 2b1. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of backlog step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 2b2. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of backlog step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- backlog item 2c. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of backlog step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- <atom id="ZVMB6Z1K" slug="a-quoted-line-for-backlog-step-3" digest="sha256:e3baf103b607b812534e45b3b4703982c61d2ce74382e99a4482b425d28d5fa8"/> -->
> A quoted line for backlog, step 3. It wraps far enough to make the blockquote bar measurable against the card border. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of backlog step 3 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- </atom-group> -->

<!-- <atom id="R09NPG66" slug="loose-ends" digest="sha256:89b45570cdca3e86e8064c9a591c2f916bc2e10a612f1501354dee402bebdc94"/> -->
## Loose ends

<!-- <atom id="A5X28J01" slug="note-0-loose-holds-at-the-current" digest="sha256:e6c59e897327dde1449762d0427f1a990c29ed8bc61b2e349062f9c6127500ce"/> -->
Note 0. loose holds at the current step. Nothing is blocked.

<!-- <atom id="XFHQSSST" slug="check-1-read-loose-step-1-md" digest="sha256:04feec4e24d94d4c7dfa2ac89df321cb14c07aac6fee84f39844999e0e0978b0"/> -->
Check 1. Read `loose/step-1.md` before the next change. The path is relative to the space root.

<!-- <atom id="691JJEH4" slug="loose-item-2a-it-is-written-long" digest="sha256:d26e29dd3da1706eec44fe1ec3246ebb6dfd7f8cf7541c3ec4d11641c0f0ac99"/> -->
- loose item 2a. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- loose item 2b. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 2b1. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 2b2. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- loose item 2c. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 2 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- <atom id="37PFYWPW" slug="a-quoted-line-for-loose-step-3" digest="sha256:91be2c8ac9be3e659ac0788254eed2a764e5b018ab45a43d7708c8167509bdac"/> -->
> A quoted line for loose, step 3. It wraps far enough to make the blockquote bar measurable against the card border. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 3 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- <atom id="NEND6SW9" slug="loose-step-4" digest="sha256:2fb0acf8d462e37bd13be1065d0211c41f7e6390470db8c7c1477b9561b41a00"/> -->
### loose step 4

<!-- <atom id="7EA90XW5" slug="sh" digest="sha256:0467d2c6a27ae7c83a60ac2d27934d2a42c4fd116d09dd78cdd6c94708962d7e"/> -->
```sh
# loose step 5
iugum wiki --port 0 ./space-5
```

<!-- <atom id="9D99ZXP3" slug="1-first-for-loose-6-it-is" digest="sha256:00191c1f469818e9079db1b98d17f6bf2886f6125c28c7c7e37b21a43430ed00"/> -->
1. first for loose 6. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 6 can be measured on the row that follows the marker rather than assumed from the stylesheet.
2. second for loose 6. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 6 can be measured on the row that follows the marker rather than assumed from the stylesheet.
3. third for loose 6. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 6 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- <atom id="SVSTMQPK" slug="a-long-reference-for-loose-a-link" digest="sha256:2810bc26b49c90e503fafdebe392e02dfb53252660810fe4ec1945eb40101753"/> -->
A long reference for loose: [a link label that is deliberately long enough to wrap inside a narrow card and reach the right border](https://example.invalid/atomdown/fixture/reference/loose/step-7?verbose=1&trace=1) and then some trailing prose.

<!-- <atom id="NZHE5V0B" slug="note-8-loose-holds-at-the-current" digest="sha256:b5662a05bc4a3c89d7164a60ff3467d7879873f1d1bd307b904397d0e211e9af"/> -->
Note 8. loose holds at the current step. Nothing is blocked.

<!-- <atom id="GV4X2NDT" slug="check-9-read-loose-step-9-md" digest="sha256:e00f0fb0851f63b22dad859270e0395b4aadc261c2c0f98fd5af15ca559637eb"/> -->
Check 9. Read `loose/step-9.md` before the next change. The path is relative to the space root.

<!-- <atom id="Y3FE57E0" slug="loose-item-10a-it-is-written-long" digest="sha256:b3445caaaea5dc5ec017bffe93a24f1ed4cef2874c2b46485de794e7f478d647"/> -->
- loose item 10a. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 10 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- loose item 10b. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 10 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 10b1. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 10 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 10b2. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 10 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- loose item 10c. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 10 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- <atom id="GENWVB2M" slug="a-quoted-line-for-loose-step-11" digest="sha256:6927268f0b3143b33790dd2ca4492240eb4e9ba7eaae0ba17ed9d5af50ba42a2"/> -->
> A quoted line for loose, step 11. It wraps far enough to make the blockquote bar measurable against the card border. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 11 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- <atom id="NY9V2Q2S" slug="loose-step-12" digest="sha256:f290b5562926b6431aab813942e6f55c8a7975fe3d7fe2ac0d924d7417fb2cf9"/> -->
### loose step 12

<!-- <atom id="A58Q0V4Y" slug="bash" digest="sha256:0d85e49824d077cd435dcf650313c9ccbc41d9e21b5ff9515a25d5451cac52e2"/> -->
```bash
# loose step 13
iugum wiki --port 0 ./space-13
```

<!-- <atom id="6YRX8S0S" slug="1-first-for-loose-14-it-is" digest="sha256:a75a9737329e66bbd74a0428bf5fa9032cb6d562f9ee044d23263441f94bdd1d"/> -->
1. first for loose 14. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 14 can be measured on the row that follows the marker rather than assumed from the stylesheet.
2. second for loose 14. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 14 can be measured on the row that follows the marker rather than assumed from the stylesheet.
3. third for loose 14. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 14 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- <atom id="JDP48M0R" slug="a-long-reference-for-loose-a-link-2" digest="sha256:0316a7d25af3786d0f9927df5f3e01f77138214fcd3c99ed086263a0d5f4e3d9"/> -->
A long reference for loose: [a link label that is deliberately long enough to wrap inside a narrow card and reach the right border](https://example.invalid/atomdown/fixture/reference/loose/step-15?verbose=1&trace=1) and then some trailing prose.

<!-- <atom id="KBM5ASVN" slug="note-16-loose-holds-at-the-current" digest="sha256:bc2e923bda653670b636d1890cbddb238ecd2f87b9bc74e90b2b760fb828bc18"/> -->
Note 16. loose holds at the current step. Nothing is blocked.

<!-- <atom id="D0V4K3RB" slug="check-17-read-loose-step-17-md" digest="sha256:28c0bccf2b9c9e7f85f6a1c139da86b4500bc54a937eddb95e98bbedf048e496"/> -->
Check 17. Read `loose/step-17.md` before the next change. The path is relative to the space root.

<!-- <atom id="RDVK8XRX" slug="loose-item-18a-it-is-written-long" digest="sha256:8010d039e1588ba8d7145422c686826e912ef0649fa492632c7513354b1a83c6"/> -->
- loose item 18a. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 18 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- loose item 18b. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 18 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 18b1. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 18 can be measured on the row that follows the marker rather than assumed from the stylesheet.
  - nested 18b2. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 18 can be measured on the row that follows the marker rather than assumed from the stylesheet.
- loose item 18c. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 18 can be measured on the row that follows the marker rather than assumed from the stylesheet.

<!-- <atom id="D597FT2E" slug="a-quoted-line-for-loose-step-19" digest="sha256:f5093b9970fac1690b2c9dd244c46e247f59934e606d40e20766a56ead281171"/> -->
> A quoted line for loose, step 19. It wraps far enough to make the blockquote bar measurable against the card border. It is written long on purpose, so this item wraps onto a second visual row at every editor width including full, and the hanging indent of loose step 19 can be measured on the row that follows the marker rather than assumed from the stylesheet.
