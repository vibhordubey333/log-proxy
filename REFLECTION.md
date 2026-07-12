# Reflection

## AI usage

I used AI (Claude) as a thinking partner to enumerate trade-offs on a handful
of genuinely ambiguous design decisions — cache concurrency strategy, the
deduplication approach, and testing strategy for the Jenkins fetch path —
before committing to one, the way I'd sanity-check a design with a senior
colleague. I made the actual decisions myself (Go + Gin, a hybrid
filesystem-plus-singleflight cache, consecutive-line dedup over block-level
or global dedup, stdlib `flag` over `cobra`, `httptest.Server` over skipping
the fetch-path tests entirely), and I wrote and debugged every line of code
myself, incrementally — building one piece at a time, compiling and running
it after each step, rather than generating the whole thing at once. Every bug below was something I found and diagnosed
myself by actually running the code, not something pre-solved for me.

**What worked well:**
- Talking through 2-3 real alternatives for each design decision before
  picking one (e.g. consecutive-line dedup vs. block-level vs. global
  non-adjacent dedup) made the trade-offs concrete and gave me a much
  clearer, more specific answer for "why did you choose this" than if I'd
  just gone with the first idea that worked.
- Designing around a `LogFetcher` interface early paid off in a way I didn't
  fully anticipate: when I discovered `ci.jenkins.io` now blocks anonymous
  access (below), adding a `FixtureFetcher` for local testing took about 15
  lines and touched no other code.
- Building incrementally and compiling/running after each small piece meant
  bugs surfaced immediately, one at a time, in a context small enough to
  actually reason about — rather than a wall of errors after writing
  everything at once.

**What didn't work well / needed correction, found while actually running
the code:**
- I initially conflated "`limit` omitted" and "`limit=0`" — both defaulted
  to "return the rest of the file," so an explicit `limit=0` silently
  returned the whole file instead of zero bytes. Caught this by deliberately
  testing the edge case, not by inspection, and fixed it with an explicit
  sentinel value distinguishing "unspecified" from "specified as zero."
- My cache directory was only created once at server startup. When I ran
  `make clean` against a server I'd left running in the background, the
  cache directory was deleted out from under the live process, and the next
  request failed with a raw `no such file or directory` instead of
  recovering. I hadn't considered that the directory could disappear
  mid-run; the fix was making the cache self-healing (`os.MkdirAll` before
  every write, not just at startup).
- Hit a `go vet` failure (`fmt.Errorf call needs 2 args but has 3 args`)
  from a format string that had silently lost a `%s` verb during an earlier
  edit — a good reminder to run `go vet` incrementally rather than only at
  the end.
- I initially assumed the leading timestamp format based on Jenkins
  Timestamper plugin documentation (`HH:mm:ss <message>`) rather than a
  captured real sample, specifically to avoid adding load to public Jenkins
  infrastructure (see below). This assumption still needs validating
  against a real log sample.

## Discovered issue: `ci.jenkins.io` now requires authentication

While manually verifying the server against the real endpoint specified in
the assignment
(`https://ci.jenkins.io/job/Core/job/jenkins/job/master/{buildId}/consoleText`),
I got a `403` from Jenkins instead of a log. Running `curl -v` directly
against Jenkins (bypassing my own server) showed the actual response
headers:

```
x-you-are-authenticated-as: anonymous
x-required-permission: hudson.model.Hudson.Read
```

Jenkins is redirecting anonymous requests to `/login` — this job's console
logs are no longer readable without an authenticated session or API token.
The assignment spec doesn't mention any authentication requirement, and
frames "public infrastructure" purely as a courtesy concern (avoid
unnecessary downloads), not an access-control one — I believe this is a
change to the real instance's access policy made after the assignment was
written, rather than something I misconfigured. I also found a Jenkins
community forum thread where an infra team member described the instance
suffering suspected LLM-driven scraping overload, which plausibly explains
why this lockdown happened.

I deliberately didn't try to work around this (scraping login flows,
guessing credentials) since that's out of scope for what this assignment is
testing. Instead:
- `JenkinsFetcher` is implemented exactly to spec and would work unmodified
  against any Jenkins instance that permits anonymous reads (or with a
  small addition of an `Authorization` header, given a valid token).
- Automated tests never depend on real Jenkins access at all — they run
  against an `httptest.Server` standing in for Jenkins.
- For manual end-to-end demonstration, I added a `FixtureFetcher`
  implementing the same `LogFetcher` interface, serving a local sample log
  file instead. Run with `make run-server-fixture`.


## What I'd do differently with more time

- Block-level or near-duplicate deduplication (e.g. collapsing repeated
  multi-line stack traces or retry loops), not just consecutive identical
  lines — likely where the real log-size wins are for genuinely noisy CI
  output, but meaningfully more complex to implement and test correctly
  than the consecutive-line approach I went with.
- A cache eviction/TTL policy — the cache currently grows unbounded, fine
  for a take-home assuming builds are immutable, but not something I'd ship
  without a retention policy.
- Validating the timestamp-stripping regex against a real captured log
  sample rather than a documentation-derived assumption.