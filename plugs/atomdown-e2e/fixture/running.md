<!-- <atomdown version="1"/> -->
<!-- <atom id="ZE5AMAB7" slug="running-todo" digest="sha256:c91bf532f29004da559d52ccea526566abeb046e7f27a1d85ad61f27cc418c17"/> -->
# Running todo

<!-- <atom id="JB9P3MD7" slug="the-master-list-daily-pages-are-filled" digest="sha256:a416cb8a06db2391c3fcc485481575192696ddf56e5e89d9a84e5c97aacb53ee"/> -->
The master list. Daily pages are filled from here, grouped by subject. This is a generated fixture: the shape of a real page, none of its content.

<!-- <atom-group id="KATZ94NM" slug="decisions"> -->

<!-- <atom id="YRCAVZM4" slug="decisions-waiting-on-me" digest="sha256:c8466bfdd91679ccde3c03501d0cba2267a18c0071df17f33f98db89c49023ac"/> -->
## Decisions waiting on me

<!-- <atom id="88D60TVF" slug="read-this-group-first-nothing-below-is" digest="sha256:20c3e5c38d903de4625aa926292a8d4fe111a249bfbc3fcd2e2afcf952464ccb"/> -->
Read this group first. Nothing below is blocked on work. It is all blocked on an answer.

<!-- <atom id="F13T5N2Z" slug="1-history-one-commit-is-still-local" digest="sha256:cc75a25aa109625d15fd71609c9198d6e0d89d04ea3a9b8e0f17fef9c967ab33"/> -->
1. **History.** One commit is still local and later commits reverse it. Drop it, squash the pair, or push the contradiction.
2. **Stale ticket.** A closed ticket carries a title that now states the opposite of the rule it closed.
3. **Rewritten policy.** An agent rewrote a conformance note to fit a change. Defensible, but read that paragraph.
4. **Editor core.** Approve a two-line change to the vendored editor, and the upstream pull request.
5. **One file, not two.** This page exists twice. Collapse to one, probably a symlink.
6. **Five calls.** Listed in the next group. Those five gate the tickets.

<!-- <atom id="M5475NQC" slug="answer-the-six-above-in-order-do" digest="sha256:162768ae07a517d47ac758224020a0179d6c7e04937283477040a5bf0e2ade1c"/> -->
Answer the six above in order. Do not start below them.

<!-- </atom-group> -->

<!-- <atom-group id="NS67J8K5" slug="resea"> -->

<!-- <atom id="X1JPRYWR" slug="resea-tickets-due-tonight" digest="sha256:922c179e1d941e64fb60191784c823faa0c121c5d785510724eb193caa65744e"/> -->
## RESEA tickets - due tonight

<!-- <atom id="CTGN89B4" slug="the-work-is-not-writing-tickets-it" digest="sha256:948176c260f9175feff7dcd90b922cd677386bef964c4f9c3f9abbe16d05ae7e"/> -->
The work is not writing tickets. It is posting a review call into the tracker. Nothing has been posted yet.

<!-- <atom id="8TTGVES7" slug="the-feature-reads-a-state-case-system" digest="sha256:c676fe912eaa7aefe84d9d4c37c7638796c34b09765e695883aabe5e0068dd23"/> -->
The feature reads a state case system through `GET /resea-status` and shows the steps in a carousel. Epic: [FFAI-62016 "Home: carousel and action plan status"](https://example.invalid/browse/FFAI-62016).

<!-- <atom id="63P82F4N" slug="ticket-state-tonight" digest="sha256:f556d2c39a77c2e0aab8ce63526a90d6f51077708699d3f3a892b9778b6e7794"/> -->
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

<!-- <atom id="AVHMMBCG" slug="five-calls-that-gate-the-tickets" digest="sha256:f3f1725f11504b69e578b1b9be9889ac0e859898621fb9024b8c19d6357c9953"/> -->
### Five calls that gate the tickets

<!-- <atom id="JP9APQ36" slug="1-the-local-note-contradicts-itself-on" digest="sha256:3b0123a2a44ff305fa1cf91c953b7d2f15ee02cd36de1bde63e47f63c5accc33"/> -->
1. The local note contradicts itself on one ticket: one section says close it as superseded, another says never close it. Pick one.
2. A request for stored state plus an error flag reverses two written decisions.
3. One ticket asks the state to drop a field another team expects to use. Settle it first.
4. Does the new admin API own the label overrides, or take upstream titles?
5. Does the P1 stay one ticket, or split now.

<!-- </atom-group> -->

<!-- <atom-group id="QP41ZR8T" slug="editor"> -->

<!-- <atom id="EH5ZNMFG" slug="editor" digest="sha256:90dfdc74de8ec936a54de91d1f15e94538e96a727b44c0de9d88bcae2a830b83"/> -->
## editor

<!-- <atom id="FVT4CR8S" slug="note-0-editor-holds-at-the-current" digest="sha256:14d32e5156be6c1abbed3e8e626363050ba7d7ad609c4182f531ec5f22904234"/> -->
Note 0. editor holds at the current step. Nothing is blocked.

<!-- <atom id="W90DMFR6" slug="check-1-read-editor-step-1-md" digest="sha256:35a85405e1cea9c6201dff5371eaaa0a2a914e63f4c24ce9c762997a3e027ed0"/> -->
Check 1. Read `editor/step-1.md` before the next change. The path is relative to the space root.

<!-- <atom id="C5YVBW6H" slug="editor-item-2a" digest="sha256:0a5aa3e124b47b072e7358ffe2da2ccdd26b56c1b450a5bf1670419d5db91fe4"/> -->
- editor item 2a
- editor item 2b
  - nested 2b1
  - nested 2b2
- editor item 2c

<!-- <atom id="V2PR2NY8" slug="a-quoted-line-for-editor-step-3" digest="sha256:6a3d351f0b8668c84359a65ab8f72212905819615670708f4b2a3b95b7689e2a"/> -->
> A quoted line for editor, step 3. It wraps far enough to make the blockquote bar measurable against the card border.

<!-- <atom id="0W91HSFT" slug="editor-step-4" digest="sha256:004b4db253805278a06bdb140d582ecb0572f79aa49397b0196ae03e373f4fa8"/> -->
### editor step 4

<!-- </atom-group> -->

<!-- <atom-group id="VD07KHM2" slug="parser"> -->

<!-- <atom id="XSC4AH8N" slug="parser" digest="sha256:52354ec7d497f980e2eae58d1e9921eaa44923773016922097c2cf6d7649324f"/> -->
## parser

<!-- <atom id="0C0YC3D1" slug="note-0-parser-holds-at-the-current" digest="sha256:f27a6072cbb763826fbbd8e4921116fa3e1196208d294aa576fde7f6d581992c"/> -->
Note 0. parser holds at the current step. Nothing is blocked.

<!-- <atom id="WWXDN53B" slug="check-1-read-parser-step-1-md" digest="sha256:12a506773433a3b3d678ebb358272cc353388e04538b29b58d812bb410f10c47"/> -->
Check 1. Read `parser/step-1.md` before the next change. The path is relative to the space root.

<!-- <atom id="5WRE7K4K" slug="parser-item-2a" digest="sha256:46f88ae149eb1a82fbfeab6acc6b812fd00cf664430d3fd3330f8416829cab77"/> -->
- parser item 2a
- parser item 2b
  - nested 2b1
  - nested 2b2
- parser item 2c

<!-- <atom id="3GGSW0FM" slug="a-quoted-line-for-parser-step-3" digest="sha256:1007dce74c01f12a91cccceed03edad11137008bf260c353150cb21ef070df44"/> -->
> A quoted line for parser, step 3. It wraps far enough to make the blockquote bar measurable against the card border.

<!-- <atom id="61APG6M7" slug="parser-step-4" digest="sha256:99ddedf5eaf0a114ba587e7de3f96dc5da69b9dfb55307ee6d178480c24763a4"/> -->
### parser step 4

<!-- </atom-group> -->

<!-- <atom-group id="WX53BCN9" slug="board-view"> -->

<!-- <atom id="J01DH8PF" slug="board-view" digest="sha256:e47f766962e8506c89940f8a88b4e1c8ff849f909293ae64a50f99840ec50b41"/> -->
## board view

<!-- <atom id="BDVX60WG" slug="note-0-board-view-holds-at-the" digest="sha256:26b01d82b944dd0d1e75bb0c850480935ab1bb19d2009b7b5b75c041fe22f67a"/> -->
Note 0. board-view holds at the current step. Nothing is blocked.

<!-- <atom id="DJW271GV" slug="check-1-read-board-view-step-1" digest="sha256:2e659246944cbed5754a78adc1a1a85102940ce849b0f95145b0b4968624f7dc"/> -->
Check 1. Read `board-view/step-1.md` before the next change. The path is relative to the space root.

<!-- <atom id="W2KXD9XY" slug="board-view-item-2a" digest="sha256:d2d4340688935656d4bec57dcdeaae014d6451df72430cc7b5eeb76c22169173"/> -->
- board-view item 2a
- board-view item 2b
  - nested 2b1
  - nested 2b2
- board-view item 2c

<!-- <atom id="TZBTMKCG" slug="a-quoted-line-for-board-view-step" digest="sha256:66a88dc5d93d2d2ded1781c8f7bfd4c9074062f1dc9689f6c4bbcf442bd3b610"/> -->
> A quoted line for board-view, step 3. It wraps far enough to make the blockquote bar measurable against the card border.

<!-- <atom id="4JW59EWQ" slug="board-view-step-4" digest="sha256:e10fc90262bf0727125031b98c95253ffb8a2f006803344cfa8fb3600d7f16d3"/> -->
### board-view step 4

<!-- </atom-group> -->

<!-- <atom-group id="YT26FGK4" slug="inline-view"> -->

<!-- <atom id="MKE7VAHN" slug="inline-view" digest="sha256:83d7d6e7b9633c305e5dbf9987434d2329ee0d0bb4cc2c6539efef9c3a006d2c"/> -->
## inline view

<!-- <atom id="R50DKZJF" slug="note-0-inline-view-holds-at-the" digest="sha256:934c6e0a0ed14e52842afe94d02b0e6a5ce90e1c5188e04cf5939d8fa2fc6d64"/> -->
Note 0. inline-view holds at the current step. Nothing is blocked.

<!-- <atom id="F1NNA3CT" slug="check-1-read-inline-view-step-1" digest="sha256:ab94e5b6063a985f0a69efb4ed075322a3fa7012c654170093c978edc3caa0d5"/> -->
Check 1. Read `inline-view/step-1.md` before the next change. The path is relative to the space root.

<!-- <atom id="6B9YH8S4" slug="inline-view-item-2a" digest="sha256:c481e7c9557dd83e111b9c1b7cd6e831f7a6d38592b27a7096a4adcc060be90b"/> -->
- inline-view item 2a
- inline-view item 2b
  - nested 2b1
  - nested 2b2
- inline-view item 2c

<!-- <atom id="2NDQEQPM" slug="a-quoted-line-for-inline-view-step" digest="sha256:defc07e58e714a74d081d48c5c60ef6431b1bc37d4f2725578a1a753b23b63b5"/> -->
> A quoted line for inline-view, step 3. It wraps far enough to make the blockquote bar measurable against the card border.

<!-- <atom id="79MJQ1PT" slug="inline-view-step-4" digest="sha256:63bfcbfa7d9cc34eaad376f5b2dac2be53485eb9688178dc90c4298048c21af4"/> -->
### inline-view step 4

<!-- </atom-group> -->

<!-- <atom-group id="ZR89PSD1" slug="database"> -->

<!-- <atom id="P519BFBA" slug="database" digest="sha256:021c8667b7dd26b8d6fd456eb044cca5d068bd42ae7df930d3bf27d9c3c43660"/> -->
## database

<!-- <atom id="707646YR" slug="note-0-database-holds-at-the-current" digest="sha256:2cc3218f845be9eea61fb81d5fc64bbe7851f10c693ba9c4c475cbca2e79749a"/> -->
Note 0. database holds at the current step. Nothing is blocked.

<!-- <atom id="09B8V7SG" slug="check-1-read-database-step-1-md" digest="sha256:e05f703fed62b0ab190ec569317a0cf25b9171025020c24f5208b6be01400545"/> -->
Check 1. Read `database/step-1.md` before the next change. The path is relative to the space root.

<!-- <atom id="9JC6046A" slug="database-item-2a" digest="sha256:d19a30c947e43ba2681ee2f0b4c06ac1b7bf4121165ea2415b130d6f9d546d29"/> -->
- database item 2a
- database item 2b
  - nested 2b1
  - nested 2b2
- database item 2c

<!-- <atom id="6EAJ2E84" slug="a-quoted-line-for-database-step-3" digest="sha256:9542e7f532afdaaa446e968d8eddd3c817d096a7936dbb780bf7f455c641a635"/> -->
> A quoted line for database, step 3. It wraps far enough to make the blockquote bar measurable against the card border.

<!-- </atom-group> -->

<!-- <atom-group id="BH14QWM7" slug="delivery"> -->

<!-- <atom id="04Y899TM" slug="delivery" digest="sha256:fa65ec268ce62fe99b12b6f220decd90f9829907924e57953a88fe274beb5be5"/> -->
## delivery

<!-- <atom id="1RMYA8T1" slug="note-0-delivery-holds-at-the-current" digest="sha256:22c984bf567661a12f588edb73c32f7f68c4d8bb20a2111991c0d83a90e4e1db"/> -->
Note 0. delivery holds at the current step. Nothing is blocked.

<!-- <atom id="ZF34QY7W" slug="check-1-read-delivery-step-1-md" digest="sha256:7ef35a2b7b5ade0ac51b6c44e7e0b1d26e7cd6493be779c42f9333f9b30ad63b"/> -->
Check 1. Read `delivery/step-1.md` before the next change. The path is relative to the space root.

<!-- <atom id="5AVF3TR7" slug="delivery-item-2a" digest="sha256:3fc564573a1ca1bdeda2583c99c321ca4268deb3cf7716ce92fcb893fe68334f"/> -->
- delivery item 2a
- delivery item 2b
  - nested 2b1
  - nested 2b2
- delivery item 2c

<!-- <atom id="0JSXVBA7" slug="a-quoted-line-for-delivery-step-3" digest="sha256:8a20c823bcbd7492e3a9b30aedaa705b59aa9049a39832be81dd32053add0381"/> -->
> A quoted line for delivery, step 3. It wraps far enough to make the blockquote bar measurable against the card border.

<!-- </atom-group> -->

<!-- <atom-group id="CJ62MVX3" slug="agents"> -->

<!-- <atom id="NJC83EZ3" slug="agents" digest="sha256:f943e43d2b5d3148d498a29c5b56666f0a0918180db58a78aba4f04510409e44"/> -->
## agents

<!-- <atom id="ZQV57PKX" slug="note-0-agents-holds-at-the-current" digest="sha256:c79a7d5af1b44e61b2d283a876ab0ea6f296dde93d27340bab86f82b28db7216"/> -->
Note 0. agents holds at the current step. Nothing is blocked.

<!-- <atom id="ZKKEWYYQ" slug="check-1-read-agents-step-1-md" digest="sha256:7a84f27914146d30cc1243a80ed985c211ee5e5172e530ca8d724bacba6f982e"/> -->
Check 1. Read `agents/step-1.md` before the next change. The path is relative to the space root.

<!-- <atom id="K9X195R3" slug="agents-item-2a" digest="sha256:cdc8e42b586b656ae7311903f122e08d28f2627f1d2800b7e2accbfbd647c5e8"/> -->
- agents item 2a
- agents item 2b
  - nested 2b1
  - nested 2b2
- agents item 2c

<!-- <atom id="633YQQ0E" slug="a-quoted-line-for-agents-step-3" digest="sha256:c55b86db06f14aa0773bbb8247ed6fb0254a5385564718e6937c1313ca741d95"/> -->
> A quoted line for agents, step 3. It wraps far enough to make the blockquote bar measurable against the card border.

<!-- </atom-group> -->

<!-- <atom-group id="DK75NRT8" slug="notes"> -->

<!-- <atom id="XQF94X1Y" slug="notes" digest="sha256:b8c01b61c7a3a84c2caa61d43ef946572b1cc7af9e856106726cb48c7d04f954"/> -->
## notes

<!-- <atom id="YSGZYT60" slug="note-0-notes-holds-at-the-current" digest="sha256:e04ae8185359cb2106ba9a6bb7d4f1eab8c2cf75bdff21f151e7e84569574f81"/> -->
Note 0. notes holds at the current step. Nothing is blocked.

<!-- <atom id="KJV1ZRGM" slug="check-1-read-notes-step-1-md" digest="sha256:26c2ab1da187617f36496f7465c8ff402b8a9d3e943c6e58bd1cdfa0e00a1961"/> -->
Check 1. Read `notes/step-1.md` before the next change. The path is relative to the space root.

<!-- <atom id="PZ2BSEX6" slug="notes-item-2a" digest="sha256:78af5266470d90211287a756b36d2beb2949b2f7c124a05e3c374897a8c4caa0"/> -->
- notes item 2a
- notes item 2b
  - nested 2b1
  - nested 2b2
- notes item 2c

<!-- <atom id="QV6M26CV" slug="a-quoted-line-for-notes-step-3" digest="sha256:2f248f6d8b286fc25dc0c287e3ff4775d40b83775f456693446eade4f2d8996b"/> -->
> A quoted line for notes, step 3. It wraps far enough to make the blockquote bar measurable against the card border.

<!-- </atom-group> -->

<!-- <atom-group id="EK38HYZ6" slug="backlog"> -->

<!-- <atom id="J04YRES6" slug="backlog" digest="sha256:5da0f9d612dc4b483a57a7a5f3e73b55b04b1cde2093b152c75f6dac3113d4cc"/> -->
## backlog

<!-- <atom id="JYZKBRNT" slug="note-0-backlog-holds-at-the-current" digest="sha256:fadbb710a360eddb24c735e07114d8e212ce66d6bb58cd32943ac61c2a296703"/> -->
Note 0. backlog holds at the current step. Nothing is blocked.

<!-- <atom id="FAKNGSJ0" slug="check-1-read-backlog-step-1-md" digest="sha256:b549c8f66edaa050032af3e4cc5404ccb2ef2b9e5a80dc51e6037a159d13187e"/> -->
Check 1. Read `backlog/step-1.md` before the next change. The path is relative to the space root.

<!-- <atom id="ZHRTZJEV" slug="backlog-item-2a" digest="sha256:b3b2b299f76f32401a21008992ab51be2d4413fe6d30d34ac5050881c83b9bdc"/> -->
- backlog item 2a
- backlog item 2b
  - nested 2b1
  - nested 2b2
- backlog item 2c

<!-- <atom id="0R1Q6QJZ" slug="a-quoted-line-for-backlog-step-3" digest="sha256:7a0e8406246fdfee7d0c78c3b39874f8da60f702d2daae76f24e04a15bb20ca8"/> -->
> A quoted line for backlog, step 3. It wraps far enough to make the blockquote bar measurable against the card border.

<!-- </atom-group> -->

<!-- <atom id="SWJQGK5Z" slug="loose-ends" digest="sha256:89b45570cdca3e86e8064c9a591c2f916bc2e10a612f1501354dee402bebdc94"/> -->
## Loose ends

<!-- <atom id="MPMYMMQ1" slug="note-0-loose-holds-at-the-current" digest="sha256:e6c59e897327dde1449762d0427f1a990c29ed8bc61b2e349062f9c6127500ce"/> -->
Note 0. loose holds at the current step. Nothing is blocked.

<!-- <atom id="KC59NS4V" slug="check-1-read-loose-step-1-md" digest="sha256:04feec4e24d94d4c7dfa2ac89df321cb14c07aac6fee84f39844999e0e0978b0"/> -->
Check 1. Read `loose/step-1.md` before the next change. The path is relative to the space root.

<!-- <atom id="44FZN698" slug="loose-item-2a" digest="sha256:e9ef409b9a9abcc433d7a2b1a5b57eb7e3eb3b7d3792e56a49f12bb87033a193"/> -->
- loose item 2a
- loose item 2b
  - nested 2b1
  - nested 2b2
- loose item 2c

<!-- <atom id="2Y0H8J24" slug="a-quoted-line-for-loose-step-3" digest="sha256:34ce6d21d111add6ea90ba78d51c901a1c5fe1e18d64c84fa3eb076aee282e12"/> -->
> A quoted line for loose, step 3. It wraps far enough to make the blockquote bar measurable against the card border.

<!-- <atom id="X20T23Q9" slug="loose-step-4" digest="sha256:eb69eccf94429169845ed213660cfb2e829e4c8daeb04ec4e6145bf05d0f144d"/> -->
### loose step 4

```sh
<!-- <atom id="NNXP9BM8" slug="loose-step-5" digest="sha256:e38b5cb2cac6762cc6855e8f8158117239093213d6bb1514e1b8bdc57ec46358"/> -->
# loose step 5
iugum wiki --port 0 ./space-5
```

<!-- <atom id="WJJWS4ZE" slug="1-first-for-loose-6" digest="sha256:73f2f2d96fe80f3fd3113b54ee9ea5590f1228f48d2f6fa6f06db44f1c1dc4d7"/> -->
1. first for loose 6
2. second for loose 6
3. third for loose 6

<!-- <atom id="082HVA13" slug="a-long-reference-for-loose-a-link" digest="sha256:2810bc26b49c90e503fafdebe392e02dfb53252660810fe4ec1945eb40101753"/> -->
A long reference for loose: [a link label that is deliberately long enough to wrap inside a narrow card and reach the right border](https://example.invalid/atomdown/fixture/reference/loose/step-7?verbose=1&trace=1) and then some trailing prose.

<!-- <atom id="R988DEJA" slug="note-8-loose-holds-at-the-current" digest="sha256:b5662a05bc4a3c89d7164a60ff3467d7879873f1d1bd307b904397d0e211e9af"/> -->
Note 8. loose holds at the current step. Nothing is blocked.

<!-- <atom id="6ADY3998" slug="check-9-read-loose-step-9-md" digest="sha256:e00f0fb0851f63b22dad859270e0395b4aadc261c2c0f98fd5af15ca559637eb"/> -->
Check 9. Read `loose/step-9.md` before the next change. The path is relative to the space root.

<!-- <atom id="3BH3856F" slug="loose-item-10a" digest="sha256:3da151375f7057f462d02f4ed5a7995a64f63f46da5c85ef60f2e82b50a59066"/> -->
- loose item 10a
- loose item 10b
  - nested 10b1
  - nested 10b2
- loose item 10c

<!-- <atom id="VZE8J4ZM" slug="a-quoted-line-for-loose-step-11" digest="sha256:cb5dc3f0ec0c0eaf363b42267c3bd07f6e3d7d526d9c8084c1098b4ba58749e9"/> -->
> A quoted line for loose, step 11. It wraps far enough to make the blockquote bar measurable against the card border.

<!-- <atom id="GDQEZPPN" slug="loose-step-12" digest="sha256:1c52dfae98e4e736685118bfff177440e1f9771e87cd3f25493cb106f709cfec"/> -->
### loose step 12

```bash
<!-- <atom id="C8D8V0A9" slug="loose-step-13" digest="sha256:6495faccb1566d063214c735b76dc211aa2d9af0ed0dc5e5006fa402da8bf02d"/> -->
# loose step 13
iugum wiki --port 0 ./space-13
```

<!-- <atom id="A4TX3496" slug="1-first-for-loose-14" digest="sha256:603872f89a57c4d0407bec64822b6037fc1c223774276365b4b78492bff602b3"/> -->
1. first for loose 14
2. second for loose 14
3. third for loose 14

<!-- <atom id="KS1YRB87" slug="a-long-reference-for-loose-a-link" digest="sha256:0316a7d25af3786d0f9927df5f3e01f77138214fcd3c99ed086263a0d5f4e3d9"/> -->
A long reference for loose: [a link label that is deliberately long enough to wrap inside a narrow card and reach the right border](https://example.invalid/atomdown/fixture/reference/loose/step-15?verbose=1&trace=1) and then some trailing prose.

<!-- <atom id="NP9896Y1" slug="note-16-loose-holds-at-the-current" digest="sha256:bc2e923bda653670b636d1890cbddb238ecd2f87b9bc74e90b2b760fb828bc18"/> -->
Note 16. loose holds at the current step. Nothing is blocked.

<!-- <atom id="JDRAK7B9" slug="check-17-read-loose-step-17-md" digest="sha256:28c0bccf2b9c9e7f85f6a1c139da86b4500bc54a937eddb95e98bbedf048e496"/> -->
Check 17. Read `loose/step-17.md` before the next change. The path is relative to the space root.

<!-- <atom id="C33WFQT9" slug="loose-item-18a" digest="sha256:7762073328d820788fcaa45a36f8ef2664170c5d5289b0e9f31e4044de1a7c98"/> -->
- loose item 18a
- loose item 18b
  - nested 18b1
  - nested 18b2
- loose item 18c

<!-- <atom id="4M2X9A97" slug="a-quoted-line-for-loose-step-19" digest="sha256:17af7ce37871d70793eb9cce3e87d70a25e4fdea348f613728043d987114e1e9"/> -->
> A quoted line for loose, step 19. It wraps far enough to make the blockquote bar measurable against the card border.
