# Live Gemini qualification

The adapter's mock-wire and saved-history tests have passed. No credentialed
Gemini qualification has run in this checkout. These checks require a credential
entered locally; do not put it in a capability, command argument, evidence export,
repository file, or chat. The workbench stores its credential only in memory.

The configured stable identifier is `gemini-3.8-flash`, with image input and
function calling documented by [Google's model reference](https://ai.google.dev/gemini-api/docs/models/gemini-3.8-flash).
The campaign ledger conservatively uses the published 2027 standard token prices,
above the current promotional rates, and reserves before every HTTP attempt.
[Google pricing](https://ai.google.dev/gemini-api/docs/pricing)

## End-to-end procedure

1. Build the pinned sources and run `./build/jarvis doctor` from the same launch
   environment as discovery. Require working accessibility and selected-window
   capture; a credential-presence flag does not prove provider access.
2. Run a live discovery against the synthetic bank:

   ```text
   ./build/jarvis discover --tenant north --task 'Observe the bank, find the member using the member_id parameter reference, search, extract Balance as USD money, and publish bank.balance at revision 1.'
   ```

3. Inspect that run's desktop journal, masked frames and public model session.
   Require, in order: scoped screenshot; actual model tool decision; admitted
   and dispatched native execution; a fresh screenshot supplied as the tool
   result; and a subsequent actual model decision. An offered tool or a compiled
   artifact alone does not prove this sequence. Account changes additionally
   require the actual human approval checkpoint.
4. Continue using the private saved session for that exact run:

   ```text
   ./build/jarvis discover --tenant north --resume-session .local/discovery/RUN_ID/session.json --task 'Continue from a fresh scoped observation. Look up the same member again and publish bank.balance_continued at revision 1 after extracting the balance.'
   ```

   Require a new live provider request after restoration and another native
   observation/tool/result cycle. Local deserialization success is insufficient.
   Private continuation files are never submission artifacts.
5. Replay the resulting immutable catalog artifact against a fresh bank process,
   supplying `scenarios/balance-m1001.contract.json` independently. Confirm zero
   model requests and a passed evidence report; use the qualification harness's
   saved-state oracle to establish the real bank outcome.

Retain exact model/prompt revision, sanitized public events, request/usage and
ledger outcomes, discovery artifact bytes, and the independent replay bundle.
Missing credentials, rate refusal, budget exhaustion or a failed stage remains
an explicit incomplete qualification. Discovery's artifact manifest intentionally
reports `not_evaluated`; publishing a proposal cannot assert task acceptance.

The existing upstream `manvi probe gemini --model gemini-3.8-flash` checks a
single tool-call wire exchange. It is useful adapter diagnosis, but is separate
from Jarvis's budgeted campaign and does not replace this screenshot/native
execution/save-and-restore sequence.
