# Pilot release decision template

Decision: **pending — no rollout approved**. Prepared documents and local automated runs do not close external acceptance gates.

| Gate | Required evidence | Status / evidence reference |
| --- | --- | --- |
| Local fixture preparation | Five portable bundles import, publish, and execute expected cases | See Phase 6 implementation record |
| Unaided use | Teammate authors, publishes, runs, and diagnoses without assistance | Open |
| Independent extension | Another developer adds an action through the documented path | Open |
| Useful recurring use | Three teammates, five useful workflows, two consecutive weeks | Open |
| Operational ownership | Named accountable operator accepts limits, recovery procedures, and escalation | Open |
| Representative load | Measured capacity, latency, backlog, and resource limits on intended installation | Open |
| Recovery and incidents | Reviewed real incident records and measured restore result | Open |
| Deployment and identity | Approved topology, provider configuration, access controls, and data handling | Open |
| Redistribution | Dependency notices and distribution license reviewed if distributing | Open |

Record the intended revision, installation, pilot dates, evidence reviewer, decision owner, accepted limitations, remaining issues, and decision date. None are assigned by this template.

Choose proceed, extend pilot, or stop only after reviewing the evidence. For each open gate, state its owner and required next evidence. Set measurable stop conditions before rollout (for example, an unsafe duplicate effect, recovery beyond the accepted limit, or lost access control). Record who stops admissions and how reopening is approved. Retain matching binaries, worker artifacts, and paired backups for the documented rollback procedure; do not downgrade a live schema.
