# test/conformance

Conformance vectors and runners: pkg/proposal rule vectors (T2.7) and the
session store driver suite (T4.2).

## pkg/proposal vectors

`proposal/*.yaml` hold the shared rule-engine corpus: each file is a YAML list
of vectors, loaded by `pkg/proposal/conformance`. A vector names the spec test
IDs it covers in `tests`, holds a proposal (its namespace defaults to
`kaddie-system`) and lists the findings static evaluation must return as
`reason`, `request` (an index into `spec.api.requests`, `-1` for the proposal
itself) and, for field rules, `field`. Findings are compared as sets; their
explanations aren't compared. Expected findings are derived from the spec, not
from running the engine. `pkg/proposal/vectors_test.go` runs the vectors tagged
with each ID, and `TestT16_ConformanceVectors` runs all of them; kaddie-tools
and kaddie-ui run the same corpus through their own engines.
