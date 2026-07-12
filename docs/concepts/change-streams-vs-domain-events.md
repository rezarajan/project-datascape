# CDC vs Domain Events

CDC events describe committed row-level state changes. Domain events describe business facts or decisions.

They may share infrastructure but must remain separate in contracts, topics, policies, retention, and processing paths.

Current release enforces this by generating separate logical streams and physical projections:

- CDC example: `EventStream/student-attendance-cdc`
- Domain example: `EventStream/attendance-corrected`

The raw archive uses separate event-class prefixes so replay and evidence can distinguish source semantics.
